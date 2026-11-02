package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sahilm/fuzzy"
)

var filterPaths = []string{
	"internal/tui/diff/model.go",
	"internal/tui/diff/model_test.go",
	"internal/review/scope.go",
	"internal/review/scope_test.go",
	"cmd/diff.go",
	"go.sum",
}

func id(s string) string { return s }

func TestFuzzyFilterExclusions(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "exclusion alone keeps order and drops matches",
			query: "!test",
			want: []string{
				"internal/tui/diff/model.go",
				"internal/review/scope.go",
				"cmd/diff.go",
				"go.sum",
			},
		},
		{
			name:  "exclusion narrows a fuzzy match",
			query: "model !test",
			want:  []string{"internal/tui/diff/model.go"},
		},
		{
			name:  "exclusions stack",
			query: "!test !review",
			want:  []string{"internal/tui/diff/model.go", "cmd/diff.go", "go.sum"},
		},
		{
			name:  "exclusion is case-insensitive",
			query: "!TEST",
			want: []string{
				"internal/tui/diff/model.go",
				"internal/review/scope.go",
				"cmd/diff.go",
				"go.sum",
			},
		},
		{
			name:  "a lone ! excludes nothing",
			query: "!",
			want:  filterPaths,
		},
		{
			name:  "! only counts at the start of a term",
			query: "go.sum!",
			want:  nil,
		},
		{
			name:  "excluding everything yields nothing",
			query: "!o",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FuzzyFilter(filterPaths, tt.query, id)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FuzzyFilter(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// Queries without a `!` must go through exactly the path they always did — the
// exclusion syntax is additive, not a rewrite of how existing filters match.
func TestFuzzyFilterUnchangedWithoutBang(t *testing.T) {
	for _, query := range []string{"", "model", "diff", "tui model", "a  b", " scope ", "zzz", "GO.SUM"} {
		t.Run(query, func(t *testing.T) {
			got := FuzzyFilter(filterPaths, query, id)
			want := legacyFuzzyFilter(filterPaths, query)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("FuzzyFilter(%q) = %v, want %v (pre-exclusion behavior)", query, got, want)
			}
		})
	}
}

// legacyFuzzyFilter is FuzzyFilter as it stood before `!` exclusions, kept as the
// reference the test above compares against.
func legacyFuzzyFilter(items []string, query string) []string {
	if query == "" {
		return items
	}
	matches := fuzzy.Find(query, items)
	filtered := make([]string, len(matches))
	for i, match := range matches {
		filtered[i] = items[match.Index]
	}
	return filtered
}

func TestSplitQuery(t *testing.T) {
	tests := []struct {
		query       string
		wantInclude string
		wantExclude []string
	}{
		{query: "model", wantInclude: "model"},
		{query: "a  b", wantInclude: "a  b"}, // no `!`: handed back verbatim, double space and all
		{query: "model !test", wantInclude: "model", wantExclude: []string{"test"}},
		{query: "!test model", wantInclude: "model", wantExclude: []string{"test"}},
		{query: "!a !b", wantInclude: "", wantExclude: []string{"a", "b"}},
		{query: "!", wantInclude: ""},
		{query: "  x   !Y  ", wantInclude: "x", wantExclude: []string{"y"}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			include, exclude := splitQuery(tt.query)
			if include != tt.wantInclude {
				t.Errorf("include = %q, want %q", include, tt.wantInclude)
			}
			if strings.Join(exclude, ",") != strings.Join(tt.wantExclude, ",") {
				t.Errorf("exclude = %v, want %v", exclude, tt.wantExclude)
			}
		})
	}
}
