package review

import (
	"reflect"
	"strings"
	"testing"
)

// projection of a RawSpec that ignores line numbers, which are asserted
// separately to keep the name/nesting cases readable.
type specKey struct {
	path string
	name string
}

func keys(raws []RawSpec) []specKey {
	out := make([]specKey, len(raws))
	for i, r := range raws {
		out[i] = specKey{path: strings.Join(r.Path, "›"), name: r.Name}
	}
	return out
}

func TestExtractors(t *testing.T) {
	tests := []struct {
		name string
		file string
		src  string
		want []specKey
	}{
		{
			name: "go subtests and leaf",
			file: "lexer_test.go",
			src: `package parse
import "testing"
func TestLex(t *testing.T) {
	t.Run("skips whitespace", func(t *testing.T) {})
	t.Run("handles unicode (ENG-2790)", func(t *testing.T) {})
}
func TestSimple(t *testing.T) {}
func helper() {}
`,
			want: []specKey{
				{"TestLex", "skips whitespace"},
				{"TestLex", "handles unicode (ENG-2790)"},
				{"", "TestSimple"},
			},
		},
		{
			name: "js nested describe/it with modifiers",
			file: "chain.test.js",
			src: `describe('chain scope', () => {
	it('keeps own cross-boundary rows', () => {})
	describe('nested', () => {
		test('builds a selected chain (ENG-2790)', () => {})
	})
	it.skip('hides opposing to_user', () => {})
})
`,
			want: []specKey{
				{"chain scope", "keeps own cross-boundary rows"},
				{"chain scope›nested", "builds a selected chain (ENG-2790)"},
				{"chain scope", "hides opposing to_user"},
			},
		},
		{
			name: "typescript template-string name",
			file: "auth.test.ts",
			src:  "describe('auth', () => {\n  it(`validates a scope`, () => {})\n})\n",
			want: []specKey{
				{"auth", "validates a scope"},
			},
		},
		{
			name: "ruby rails test macro, rspec nesting, and def test_",
			file: "backfill_test.rb",
			src: `class BackfillTest < ActiveSupport::TestCase
  test 'converts content and creates version (ENG-2794)' do
    assert true
  end

  describe 'snapshot path' do
    context 'when blank' do
      it 'falls back to content_html' do
      end
    end
  end

  def test_legacy_helper
  end
end
`,
			want: []specKey{
				{"BackfillTest", "converts content and creates version (ENG-2794)"},
				{"BackfillTest›snapshot path›when blank", "falls back to content_html"},
				{"BackfillTest", "test_legacy_helper"},
			},
		},
		{
			name: "python module func and Test class methods",
			file: "test_backfill.py",
			src: `def test_module_level():
    pass

class TestThing:
    def test_method(self):
        pass

    def helper(self):
        pass
`,
			want: []specKey{
				{"", "test_module_level"},
				{"TestThing", "test_method"},
			},
		},
		{
			name: "rust #[test] and #[tokio::test], skipping cfg and plain fns",
			file: "lib.rs",
			src: `#[cfg(test)]
mod tests {
    #[test]
    fn adds_two() {}

    #[tokio::test]
    async fn async_case() {}

    fn not_a_test() {}
}
`,
			want: []specKey{
				{"", "adds_two"},
				{"", "async_case"},
			},
		},
		{
			name: "rust test attribute separated from fn by a comment",
			file: "lib.rs",
			src:  "#[test]\n// explains the case\nfn adds_two() {}\n",
			want: []specKey{
				{"", "adds_two"},
			},
		},
		{
			name: "js leading comment before the test label",
			file: "x.test.js",
			src:  "describe('scope', () => {\n  it(/* ENG-1 */ 'keeps rows', () => {})\n})\n",
			want: []specKey{
				{"scope", "keeps rows"},
			},
		},
		{
			name: "go bodyless test decl is skipped, not a panic",
			file: "stub_test.go",
			// A valid bodyless test func (asm/linkname stub) must not SIGSEGV the
			// subtest walk; it's skipped and the real test still surfaces.
			src: `package p
import "testing"
func TestExternal(t *testing.T)
func TestReal(t *testing.T) {}
`,
			want: []specKey{
				{"", "TestReal"},
			},
		},
		{
			name: "go nested t.Run threads the enclosing path",
			file: "nested_test.go",
			src: `package p
import "testing"
func TestX(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) {})
	})
}
`,
			want: []specKey{
				{"TestX", "outer"},
				{"TestX›outer", "inner"},
			},
		},
		{
			name: "go keyed-struct table-driven cases",
			file: "parser_test.go",
			src: `package p
import "testing"
func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "handles empty input", in: ""},
		{name: "parses a nested block (ENG-42)", in: "{}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}
`,
			want: []specKey{
				{"TestParse", "handles empty input"},
				{"TestParse", "parses a nested block (ENG-42)"},
			},
		},
		{
			name: "go map-keyed table and positional table",
			file: "x_test.go",
			src: `package p
import "testing"
func TestMap(t *testing.T) {
	cases := map[string]struct{ in string }{
		"alpha case": {in: "a"},
		"beta case":  {in: "b"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) { _ = tc })
	}
}
func TestPositional(t *testing.T) {
	for _, tt := range []struct {
		desc string
		in   string
	}{
		{"first positional", "x"},
		{"second positional", "y"},
	} {
		t.Run(tt.desc, func(t *testing.T) {})
	}
}
`,
			want: []specKey{
				{"TestMap", "alpha case"},
				{"TestMap", "beta case"},
				{"TestPositional", "first positional"},
				{"TestPositional", "second positional"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := extractorFor(tt.file)
			if ext == nil {
				t.Fatalf("no extractor for %s", tt.file)
			}
			raws := ext.Extract([]byte(tt.src))
			if got := keys(raws); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("specs mismatch\n got: %+v\nwant: %+v", got, tt.want)
			}
			for _, r := range raws {
				if r.StartLine <= 0 || r.EndLine < r.StartLine {
					t.Errorf("bad line range for %q: [%d,%d]", r.Name, r.StartLine, r.EndLine)
				}
			}
		})
	}
}

// TestExtractorForGating covers item 8: extractors run only on conventional
// test-file names (Rust stays attribute-gated, so any .rs file gets one), so a
// production file's `func TestConnection(...)` doesn't pollute the lens.
func TestExtractorForGating(t *testing.T) {
	tests := []struct {
		path string
		want bool // whether an extractor is returned
	}{
		{"internal/net/conn.go", false},
		{"internal/net/conn_test.go", true},
		{"src/app.js", false},
		{"src/app.test.js", true},
		{"src/app.spec.ts", true},
		{"src/__tests__/app.js", true},
		{"src/util.py", false},
		{"test_util.py", true},
		{"util_test.py", true},
		{"models/user.rb", false},
		{"models/user_test.rb", true},
		{"models/user_spec.rb", true},
		{"src/lib.rs", true}, // attribute-gated, not filename-gated
	}
	for _, tt := range tests {
		if got := extractorFor(tt.path) != nil; got != tt.want {
			t.Errorf("extractorFor(%q) != nil = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// TestExtractRobustness covers item 1b: a tree-sitter Extract on malformed input
// must degrade to zero results, never panic (the whole `rift diff --tests` would
// otherwise crash).
func TestExtractRobustness(t *testing.T) {
	ext := extractorFor("broken.test.js")
	if ext == nil {
		t.Fatal("no js extractor")
	}
	for _, src := range []string{"", "describe('unterminated", "\x00\x01\x02 garbage {{{", "it("} {
		_ = ext.Extract([]byte(src)) // must not panic
	}
}

func TestAddedLines(t *testing.T) {
	raw := `diff --git a/x.js b/x.js
--- a/x.js
+++ b/x.js
@@ -1,2 +1,4 @@
 context line
+added one
+added two
 trailing context
`
	added := addedLinesByPath(raw)["x.js"]
	if !added[2] || !added[3] {
		t.Errorf("expected lines 2,3 added, got %v", added)
	}
	if added[1] || added[4] {
		t.Errorf("context lines must not be marked added, got %v", added)
	}
}

// TestDeletionMarks covers item 5's threat model: a hunk that only deletes lines
// (e.g. removing an assertion) must still mark a new-side position so the weakened
// test surfaces, rather than producing an empty added set.
func TestDeletionMarks(t *testing.T) {
	raw := `diff --git a/x_test.go b/x_test.go
--- a/x_test.go
+++ b/x_test.go
@@ -10,4 +10,3 @@
 keep one
-removed assertion
 keep two
 keep three
`
	added := addedLinesByPath(raw)["x_test.go"]
	// The deletion abuts new-side line 11 (the line now following the deleted one).
	if !added[11] {
		t.Errorf("deletion should mark its new-side position (11), got %v", added)
	}
}

// TestClassify covers the change kinds a touched spec can have, decided from
// which of its lines the diff added plus the prior test names: a wholly new test,
// a rename (declaration touched, name new to the file), a body-only modification,
// and — the item 9 fix — a declaration touch that keeps an existing name (e.g.
// adding `.only`), which is a modification, not a rename.
func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		spec     RawSpec
		added    map[int]bool
		oldNames map[string]bool
		want     string
	}{
		{"all lines new", RawSpec{Name: "t", StartLine: 10, EndLine: 13}, map[int]bool{10: true, 11: true, 12: true, 13: true}, nil, "added"},
		{"declaration only, new name", RawSpec{Name: "renamed", StartLine: 10, EndLine: 13}, map[int]bool{10: true}, nil, "renamed"},
		{"declaration plus body, new name", RawSpec{Name: "renamed", StartLine: 10, EndLine: 13}, map[int]bool{10: true, 12: true}, nil, "renamed"},
		{"body only", RawSpec{Name: "t", StartLine: 10, EndLine: 13}, map[int]bool{12: true}, nil, "modified"},
		{"declaration touch, name unchanged", RawSpec{Name: "keeps", StartLine: 10, EndLine: 13}, map[int]bool{10: true}, map[string]bool{"keeps": true}, "modified"},
	}
	for _, tt := range tests {
		if got := classify(tt.spec, tt.added, tt.oldNames); got != tt.want {
			t.Errorf("%s: classify = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestAllLines covers the whole-file added set used for a root-commit/EmptyTree
// base: every line counts, blank lines included (so a new test with a blank
// interior line classifies as added, not renamed), but a trailing newline does
// not invent a phantom line past the end.
func TestAllLines(t *testing.T) {
	got := allLines([]byte("a\n\nb\n"))
	for _, n := range []int{1, 2, 3} {
		if !got[n] {
			t.Errorf("line %d should be marked, got %v", n, got)
		}
	}
	if got[4] {
		t.Errorf("trailing newline invented a phantom line 4: %v", got)
	}
	if len(allLines(nil)) != 0 {
		t.Errorf("empty content should mark no lines")
	}
}
