package review

import (
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// RawSpec is a test declaration found in a single file, before it's correlated
// with the diff. Lines are 1-based and refer to the parsed (new-side) content.
type RawSpec struct {
	Path      []string // describe/context nesting, outermost first
	Name      string
	StartLine int
	EndLine   int
}

// Extractor pulls test declarations out of a file's source. Each implementation
// uses whatever parser fits its language best (go/ast for Go, tree-sitter for
// the rest); the diff-correlation layer above treats them uniformly.
type Extractor interface {
	Language() string
	Extract(src []byte) []RawSpec
}

// extractorFor returns the extractor for a path's extension, or nil if the
// language isn't supported or the file isn't a conventional test file (the file
// is then skipped, per graceful degradation). Gating on test-file names keeps a
// production `func TestConnection(host string) error` out of the tests lens. Rust
// is the exception: its tests live inline in production files, so it stays gated
// by the #[test] attribute in rustClassify rather than by filename.
func extractorFor(path string) Extractor {
	base := strings.ToLower(filepath.Base(path))
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		if strings.HasSuffix(base, "_test.go") {
			return goExtractor{}
		}
	case ".js", ".jsx", ".mjs", ".cjs":
		if isJSTestFile(path, base) {
			return tsExt("javascript", grammars.JavascriptLanguage, jsClassify)
		}
	case ".ts", ".mts", ".cts":
		if isJSTestFile(path, base) {
			return tsExt("typescript", grammars.TypescriptLanguage, jsClassify)
		}
	case ".tsx":
		if isJSTestFile(path, base) {
			return tsExt("typescript", grammars.TsxLanguage, jsClassify)
		}
	case ".rb":
		if hasSuffixAny(base, "_test.rb", "_spec.rb") {
			return tsExt("ruby", grammars.RubyLanguage, rubyClassify)
		}
	case ".py":
		if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
			return tsExt("python", grammars.PythonLanguage, pythonClassify)
		}
	case ".rs":
		return tsExt("rust", grammars.RustLanguage, rustClassify)
	}
	return nil
}

// isJSTestFile matches the JS/TS test conventions: a *.test.* / *.spec.* file, or
// any file under a __tests__/ directory.
func isJSTestFile(path, base string) bool {
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if seg == "__tests__" {
			return true
		}
	}
	return false
}

func hasSuffixAny(s string, suffixes ...string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

// tsExt builds a tree-sitter extractor, returning nil (so the file is skipped)
// when the grammar isn't embedded in this build. gotreesitter panics on a
// missing grammar blob; recovering keeps the graceful-degradation contract if
// the build's grammar_subset tags ever drift from the languages above.
func tsExt(language string, load func() *gts.Language, c classifier) Extractor {
	lang := func() (l *gts.Language) {
		defer func() { _ = recover() }()
		return load()
	}()
	if lang == nil {
		return nil
	}
	return tsExtractor{language, lang, c}
}

// goExtractor finds Go test entry points (Test/Fuzz/Example funcs) and their
// t.Run subtests via the standard library parser.
type goExtractor struct{}

func (goExtractor) Language() string { return "go" }

func (goExtractor) Extract(src []byte) []RawSpec {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "", src, 0)
	if err != nil {
		return nil
	}
	line := func(p token.Pos) int { return fset.Position(p).Line }

	var out []RawSpec
	for _, d := range f.Decls {
		fn, ok := d.(*goast.FuncDecl)
		if !ok || fn.Recv != nil || !isGoTestFunc(fn.Name.Name) {
			continue
		}
		if fn.Body == nil {
			continue // a bodyless test decl (asm/linkname stub): nothing to walk
		}
		// A test with subtests is represented by its subtests; one without is a
		// leaf in its own right.
		if subs := goSubtests(fn, line); len(subs) > 0 {
			out = append(out, subs...)
		} else {
			out = append(out, RawSpec{
				Name:      fn.Name.Name,
				StartLine: line(fn.Pos()),
				EndLine:   line(fn.End()),
			})
		}
	}
	return out
}

// goSubtests collects a Test function's subtests: literal t.Run("name", …) calls
// and table-driven cases, where t.Run(tt.field, …) ranges a slice of structs or
// t.Run(key, …) ranges a map. Table case names are pulled from the table literal.
// It threads the enclosing t.Run nesting into each spec's Path, so an inner
// t.Run reads as `TestX › outer › inner` rather than being flattened onto TestX.
func goSubtests(fn *goast.FuncDecl, line func(token.Pos) int) []RawSpec {
	var subs []RawSpec
	spec := func(name string, n goast.Node, path []string) RawSpec {
		return RawSpec{Path: append([]string{}, path...), Name: name, StartLine: line(n.Pos()), EndLine: line(n.End())}
	}

	var walk func(n goast.Node, path []string)
	walk = func(n goast.Node, path []string) {
		goast.Inspect(n, func(node goast.Node) bool {
			switch x := node.(type) {
			case *goast.RangeStmt:
				for _, c := range tableCases(fn.Body, x) {
					subs = append(subs, spec(c.name, c.node, path))
				}
			case *goast.CallExpr:
				name, ok := runStringLit(x)
				if !ok {
					return true
				}
				subs = append(subs, spec(name, x, path))
				// Descend into the subtest body under the extended path; return
				// false so Inspect doesn't re-walk it and drop the nesting.
				if body := runFuncLitBody(x); body != nil {
					walk(body, append(append([]string{}, path...), name))
				}
				return false
			}
			return true
		})
	}
	walk(fn.Body, []string{fn.Name.Name})
	return subs
}

// runFuncLitBody returns the body of the func-literal argument to a t.Run call,
// which holds any nested subtests, or nil if the subtest fn isn't a literal.
func runFuncLitBody(call *goast.CallExpr) *goast.BlockStmt {
	for _, a := range call.Args {
		if lit, ok := a.(*goast.FuncLit); ok {
			return lit.Body
		}
	}
	return nil
}

// runStringLit returns the literal name of a `<x>.Run("name", …)` call.
func runStringLit(call *goast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*goast.SelectorExpr)
	if !ok || sel.Sel.Name != "Run" || len(call.Args) == 0 {
		return "", false
	}
	return goStringLit(call.Args[0])
}

type tableCase struct {
	name string
	node goast.Node // the table row, for line correlation against the diff
}

// tableCases extracts subtest names from a `for … := range table` loop whose body
// calls t.Run with the range variable: a slice of structs (t.Run(tt.field, …)) or
// a map keyed by name (t.Run(key, …)).
func tableCases(body *goast.BlockStmt, rng *goast.RangeStmt) []tableCase {
	field, useKey := runRangeField(rng)
	if field == "" && !useKey {
		return nil
	}
	table := resolveComposite(body, rng.X)
	if table == nil {
		return nil
	}

	var cases []tableCase
	if useKey { // map[string]T{ "case": {…} } → names are the keys
		for _, e := range table.Elts {
			if kv, ok := e.(*goast.KeyValueExpr); ok {
				if name, ok := goStringLit(kv.Key); ok {
					cases = append(cases, tableCase{name, kv})
				}
			}
		}
		return cases
	}

	idx := fieldIndex(table.Type, field) // for positional rows {"case", …}
	for _, e := range table.Elts {
		row, ok := e.(*goast.CompositeLit)
		if !ok {
			continue
		}
		if name, ok := rowFieldString(row, field, idx); ok {
			cases = append(cases, tableCase{name, row})
		}
	}
	return cases
}

// runRangeField inspects a range loop's body for a t.Run call and reports how the
// case name is sourced: the struct field of the value variable (t.Run(tt.name,…)),
// or, with useKey, the map-key variable (t.Run(name,…)).
func runRangeField(rng *goast.RangeStmt) (field string, useKey bool) {
	val, key := identName(rng.Value), identName(rng.Key)
	goast.Inspect(rng.Body, func(n goast.Node) bool {
		call, ok := n.(*goast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*goast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" || len(call.Args) == 0 {
			return true
		}
		switch a := call.Args[0].(type) {
		case *goast.SelectorExpr:
			if val != "" && identName(a.X) == val {
				field = a.Sel.Name
			}
		case *goast.Ident:
			if key != "" && a.Name == key {
				useKey = true
			}
		}
		return true
	})
	return field, useKey
}

// resolveComposite returns the composite literal x refers to: x itself, or the
// value of the `x := {…}` / `var x = {…}` that defines it within body.
func resolveComposite(body *goast.BlockStmt, x goast.Expr) *goast.CompositeLit {
	if lit, ok := x.(*goast.CompositeLit); ok {
		return lit
	}
	name := identName(x)
	if name == "" {
		return nil
	}
	var found *goast.CompositeLit
	goast.Inspect(body, func(n goast.Node) bool {
		if found != nil {
			return false
		}
		switch s := n.(type) {
		case *goast.AssignStmt:
			for i, lhs := range s.Lhs {
				if identName(lhs) == name && i < len(s.Rhs) {
					if lit, ok := s.Rhs[i].(*goast.CompositeLit); ok {
						found = lit
					}
				}
			}
		case *goast.ValueSpec:
			for i, nm := range s.Names {
				if nm.Name == name && i < len(s.Values) {
					if lit, ok := s.Values[i].(*goast.CompositeLit); ok {
						found = lit
					}
				}
			}
		}
		return true
	})
	return found
}

// rowFieldString returns a struct-literal row's value for field, by key
// ({field: "x"}) or, falling back to idx, by position ({"x", …}).
func rowFieldString(row *goast.CompositeLit, field string, idx int) (string, bool) {
	for _, e := range row.Elts {
		if kv, ok := e.(*goast.KeyValueExpr); ok {
			if identName(kv.Key) == field {
				return goStringLit(kv.Value)
			}
		}
	}
	if idx >= 0 && idx < len(row.Elts) {
		if _, keyed := row.Elts[0].(*goast.KeyValueExpr); !keyed {
			return goStringLit(row.Elts[idx])
		}
	}
	return "", false
}

// fieldIndex returns the position of field in a []struct{…} element type, or -1.
func fieldIndex(typ goast.Expr, field string) int {
	arr, ok := typ.(*goast.ArrayType)
	if !ok {
		return -1
	}
	st, ok := arr.Elt.(*goast.StructType)
	if !ok {
		return -1
	}
	i := 0
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 { // embedded field
			if identName(f.Type) == field {
				return i
			}
			i++
			continue
		}
		for _, nm := range f.Names {
			if nm.Name == field {
				return i
			}
			i++
		}
	}
	return -1
}

func identName(e goast.Expr) string {
	if id, ok := e.(*goast.Ident); ok && id.Name != "_" {
		return id.Name
	}
	return ""
}

func isGoTestFunc(name string) bool {
	for _, p := range []string{"Test", "Fuzz", "Example", "Benchmark"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func goStringLit(e goast.Expr) (string, bool) {
	lit, ok := e.(*goast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// tsNode binds a gotreesitter node to its language so the classifiers can read
// types, fields, and text without threading the language through every call.
type tsNode struct {
	n    *gts.Node
	lang *gts.Language
}

func (w tsNode) ok() bool                 { return w.n != nil }
func (w tsNode) typ() string              { return w.n.Type(w.lang) }
func (w tsNode) text(s []byte) string     { return w.n.Text(s) }
func (w tsNode) namedChildCount() int     { return w.n.NamedChildCount() }
func (w tsNode) namedChild(i int) tsNode  { return tsNode{w.n.NamedChild(i), w.lang} }
func (w tsNode) field(name string) tsNode { return tsNode{w.n.ChildByFieldName(name, w.lang), w.lang} }
func (w tsNode) startRow() int            { return int(w.n.StartPoint().Row) }
func (w tsNode) endRow() int              { return int(w.n.EndPoint().Row) }

// prevNamedSibling returns the nearest preceding named sibling (gotreesitter has
// no direct accessor), used to find a Rust function's attributes.
func (w tsNode) prevNamedSibling() tsNode {
	for s := w.n.PrevSibling(); s != nil; s = s.PrevSibling() {
		if s.IsNamed() {
			return tsNode{s, w.lang}
		}
	}
	return tsNode{nil, w.lang}
}

// tsExtractor handles any tree-sitter language whose test idioms can be matched
// by a per-language classifier: the describe/it family (Jest, Vitest, Mocha,
// RSpec), Rails/Minitest `test "name"` macros, pytest, and Rust #[test] fns.
type tsExtractor struct {
	language string
	lang     *gts.Language
	classify classifier
}

func (e tsExtractor) Language() string { return e.language }

func (e tsExtractor) Extract(src []byte) (specs []RawSpec) {
	// The grammar-load path recovers in tsExt; the parse/walk path can panic too
	// (a malformed tree, a grammar edge case). Graceful degradation: yield nothing
	// rather than crash the whole `rift diff --tests`.
	defer func() {
		if recover() != nil {
			specs = nil
		}
	}()
	tree, err := gts.NewParser(e.lang).Parse(src)
	if err != nil || tree == nil || tree.RootNode() == nil {
		return nil
	}
	return walkTree(tsNode{tree.RootNode(), e.lang}, src, e.classify)
}

type nodeKind int

const (
	kindNone nodeKind = iota
	kindDescribe
	kindTest
)

// classifier inspects a node and reports whether it opens a describe/context
// container (nesting the path) or declares a test (a leaf), plus its label.
type classifier func(n tsNode, src []byte) (nodeKind, string)

// walkTree descends the syntax tree, threading the describe/context nesting into
// each test's Path. A test node is not descended into, so calls in its body
// aren't mistaken for tests.
func walkTree(root tsNode, src []byte, classify classifier) []RawSpec {
	var out []RawSpec
	var walk func(n tsNode, path []string)
	walk = func(n tsNode, path []string) {
		switch kind, label := classify(n, src); kind {
		case kindDescribe:
			walkChildren(n, append(append([]string{}, path...), label), walk)
			return
		case kindTest:
			out = append(out, RawSpec{
				Path:      append([]string{}, path...),
				Name:      label,
				StartLine: n.startRow() + 1,
				EndLine:   n.endRow() + 1,
			})
			return
		}
		walkChildren(n, path, walk)
	}
	walk(root, nil)
	return out
}

func walkChildren(n tsNode, path []string, walk func(tsNode, []string)) {
	for i := 0; i < n.namedChildCount(); i++ {
		walk(n.namedChild(i), path)
	}
}

// jsClassify matches the JS/TS describe/it family: call_expression nodes whose
// callee resolves to describe/it/test (peeling .only/.each/.skip).
func jsClassify(n tsNode, src []byte) (nodeKind, string) {
	if n.typ() != "call_expression" {
		return kindNone, ""
	}
	callee := calleeName(n.field("function"), src)
	label, ok := firstStringArg(n, src)
	switch {
	case ok && isDescribe(callee):
		return kindDescribe, label
	case ok && isTest(callee):
		return kindTest, label
	}
	return kindNone, ""
}

// rubyClassify matches RSpec describe/context/it, Rails `test "name" do` macros,
// Minitest `def test_*` methods, and uses the enclosing class as the nesting
// container (so a Rails `class FooControllerTest` prefaces its `test` cases).
func rubyClassify(n tsNode, src []byte) (nodeKind, string) {
	switch n.typ() {
	case "class":
		return kindDescribe, lastConst(fieldText(n, "name", src))
	case "call":
		method := fieldText(n, "method", src)
		label, ok := firstStringArg(n, src)
		switch {
		case ok && isDescribe(method):
			return kindDescribe, label
		case ok && isTest(method):
			return kindTest, label
		}
	case "method": // def test_something
		if name := fieldText(n, "name", src); strings.HasPrefix(name, "test_") {
			return kindTest, name
		}
	}
	return kindNone, ""
}

// lastConst drops a Ruby constant's namespace, so Api::V1::FooTest reads as FooTest.
func lastConst(name string) string {
	if i := strings.LastIndex(name, "::"); i >= 0 {
		return name[i+2:]
	}
	return name
}

// pythonClassify matches pytest/unittest: `Test*` classes nest the path, and
// `test*` functions (module-level or methods) are leaves.
func pythonClassify(n tsNode, src []byte) (nodeKind, string) {
	switch n.typ() {
	case "class_definition":
		if name := fieldText(n, "name", src); strings.HasPrefix(name, "Test") {
			return kindDescribe, name
		}
	case "function_definition":
		if name := fieldText(n, "name", src); strings.HasPrefix(name, "test") {
			return kindTest, name
		}
	}
	return kindNone, ""
}

// rustClassify matches functions carrying a #[test]-family attribute (including
// #[tokio::test]), which sit as attribute_item siblings preceding the function.
// A #[cfg(test)] guard is not a test attribute (its path is cfg, not test).
func rustClassify(n tsNode, src []byte) (nodeKind, string) {
	if n.typ() == "function_item" && hasTestAttr(n, src) {
		return kindTest, fieldText(n, "name", src)
	}
	return kindNone, ""
}

func hasTestAttr(fn tsNode, src []byte) bool {
	for s := fn.prevNamedSibling(); s.ok(); s = s.prevNamedSibling() {
		switch {
		case isComment(s.typ()):
			continue // a comment may sit between an attribute and its function
		case s.typ() == "attribute_item":
			if attrIsTest(s, src) {
				return true
			}
		default:
			return false // a non-attribute node ends the attribute run
		}
	}
	return false
}

// attrIsTest reports whether an attribute_item's path ends in `test`, matching
// #[test] and #[tokio::test] but not #[cfg(test)] (path cfg, test is an arg).
func attrIsTest(attrItem tsNode, src []byte) bool {
	attr := attrItem.namedChild(0)
	if !attr.ok() || attr.typ() != "attribute" {
		return false
	}
	switch path := attr.namedChild(0); {
	case !path.ok():
		return false
	case path.typ() == "identifier":
		return path.text(src) == "test"
	case path.typ() == "scoped_identifier":
		return fieldText(path, "name", src) == "test"
	}
	return false
}

func fieldText(n tsNode, field string, src []byte) string {
	if c := n.field(field); c.ok() {
		return c.text(src)
	}
	return ""
}

// calleeName returns the base callee identifier, peeling member accesses so
// it.only, describe.each, test.skip all resolve to it/describe/test.
func calleeName(fn tsNode, src []byte) string {
	if !fn.ok() {
		return ""
	}
	switch fn.typ() {
	case "identifier":
		return fn.text(src)
	case "member_expression":
		return calleeName(fn.field("object"), src)
	}
	return ""
}

func firstStringArg(call tsNode, src []byte) (string, bool) {
	args := call.field("arguments")
	if !args.ok() {
		return "", false
	}
	for i := 0; i < args.namedChildCount(); i++ {
		arg := args.namedChild(i)
		if isComment(arg.typ()) {
			continue // skip a leading comment, e.g. it(/* note */ 'name', ...)
		}
		switch arg.typ() {
		case "string", "template_string":
			return trimQuotes(arg.text(src)), true
		}
		return "", false // first real argument isn't a string → no static label
	}
	return "", false
}

func isComment(typ string) bool {
	switch typ {
	case "comment", "line_comment", "block_comment":
		return true
	}
	return false
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		switch s[0] {
		case '\'', '"', '`':
			if s[len(s)-1] == s[0] {
				return s[1 : len(s)-1]
			}
		}
	}
	return s
}

func isDescribe(callee string) bool {
	switch callee {
	case "describe", "context", "suite":
		return true
	}
	return false
}

func isTest(callee string) bool {
	switch callee {
	case "it", "test", "specify", "example", "scenario":
		return true
	}
	return false
}
