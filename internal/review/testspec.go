package review

import (
	"regexp"
	"sort"
	"strings"

	"github.com/madhermit/rift/internal/git"
)

// Spec is a single test case touched by the diff under review: its name, the
// describe/context path that contains it, and how the diff changed it.
type Spec struct {
	File     string   `json:"file"`
	OldPath  string   `json:"old_path,omitempty"` // pre-image path when the file was renamed in scope
	Language string   `json:"language"`
	Path     []string `json:"path,omitempty"`
	Name     string   `json:"name"`
	Line     int      `json:"line"`
	Status   string   `json:"status"` // "added" | "renamed" | "modified"
	Ticket   string   `json:"ticket,omitempty"`
}

// Glyph is the tests lens's own one-character change-kind marker: + a wholly new
// test, → a renamed test (its name changed), ~ a test whose body changed under
// an unchanged name (the last being the one a reviewer most wants to scrutinise,
// since an agent can quietly weaken an assertion there). It deliberately doesn't
// reuse git's file-status letters (A/R/M): a spec is a test case, not a file, and
// the same diff can mark a file Modified while a test inside it is new.
func (s Spec) Glyph() string {
	switch s.Status {
	case "added":
		return "+"
	case "renamed":
		return "→"
	default:
		return "~"
	}
}

// PathPrefix is the spec's describe/context nesting rendered as a "a › b › "
// prefix (empty when there's no nesting), so a renderer can place it before the
// name.
func (s Spec) PathPrefix() string {
	if len(s.Path) == 0 {
		return ""
	}
	return strings.Join(s.Path, " › ") + " › "
}

// DiffScope selects the diff the tests are read from: a working-tree change
// (optionally the staged side), or a committed range Base..Target.
type DiffScope struct {
	Staged bool
	Base   string   // "" for a working-tree diff
	Target string   // "" for a working-tree diff; a ref for a committed diff
	Paths  []string // optional pathspec filter (from `-- path...`)
}

// Committed reports whether the scope is a committed range rather than a
// working-tree (or staged) diff.
func (s DiffScope) Committed() bool { return s.Target != "" }

// ticketPattern matches conventional issue keys embedded in test names, e.g.
// ENG-2790 or ABC-12.
var ticketPattern = regexp.MustCompile(`[A-Z][A-Z0-9]+-\d+`)

// Collect extracts the test cases the diff touches, correlating each file's
// parsed specs with the lines the diff changed.
func Collect(repo *git.Repo, scope DiffScope) ([]Spec, error) {
	scopes, err := gather(repo, scope)
	if err != nil {
		return nil, err
	}

	var specs []Spec
	for _, sc := range scopes {
		oldNames := extractNames(sc.Ext, sc.Old)
		for _, r := range sc.Ext.Extract(sc.Content) {
			if !touched(r, sc.Added) {
				continue
			}
			specs = append(specs, Spec{
				File:     sc.Path,
				OldPath:  sc.OldPath,
				Language: sc.Ext.Language(),
				Path:     r.Path,
				Name:     r.Name,
				Line:     r.StartLine,
				Status:   classify(r, sc.Added, oldNames),
				Ticket:   ticketPattern.FindString(r.Name),
			})
		}
	}

	sort.Slice(specs, func(i, j int) bool {
		if specs[i].File != specs[j].File {
			return specs[i].File < specs[j].File
		}
		return specs[i].Line < specs[j].Line
	})
	return specs, nil
}

func touched(r RawSpec, added map[int]bool) bool {
	for l := r.StartLine; l <= r.EndLine; l++ {
		if added[l] {
			return true
		}
	}
	return false
}

// extractNames returns the set of test names present in the old-side content,
// used to distinguish a genuine rename from a declaration-line touch that leaves
// the name unchanged. Nil/empty content (a new or root-commit file) → no names.
func extractNames(ext Extractor, old []byte) map[string]bool {
	if len(old) == 0 {
		return nil
	}
	names := make(map[string]bool)
	for _, r := range ext.Extract(old) {
		names[r.Name] = true
	}
	return names
}

// classify decides a touched spec's change kind from the added-line set and the
// prior (old-side) test names: every line added → a wholly new test; a touched
// declaration line whose name is new to the file → a rename of an existing test;
// anything else (body-only change, or a declaration touch that keeps an existing
// name, e.g. adding `.only`) → a modification under an unchanged name.
func classify(r RawSpec, added map[int]bool, oldNames map[string]bool) string {
	switch {
	case allAdded(r, added):
		return "added"
	case added[r.StartLine] && !oldNames[r.Name]:
		return "renamed"
	default:
		return "modified"
	}
}

func allAdded(r RawSpec, added map[int]bool) bool {
	for l := r.StartLine; l <= r.EndLine; l++ {
		if !added[l] {
			return false
		}
	}
	return true
}
