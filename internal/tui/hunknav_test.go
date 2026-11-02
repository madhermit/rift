package tui

import (
	"fmt"
	"testing"

	"github.com/madhermit/rift/internal/diff"
)

// The git engine renders the unified diff verbatim, so a hunk is located by its
// own @@ header — exactly, and without depending on any line-number gutter.
func TestHunkOffsets_GitHeaders(t *testing.T) {
	lines := []string{
		"diff --git a/f.go b/f.go",
		"--- a/f.go",
		"+++ b/f.go",
		"@@ -1,7 +1,7 @@",
		"package main",
		"	return 11",
		"@@ -9,7 +9,7 @@ func beta() int {",
		"}",
		"	return 33",
	}
	hunks := []diff.Hunk{
		{Header: "@@ -1,7 +1,7 @@", NewStart: 1},
		{Header: "@@ -9,7 +9,7 @@ func beta() int {", NewStart: 9},
	}
	got := HunkOffsets(lines, hunks)
	want := []int{3, 6}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("offset %d = %d, want %d", i, got[i], want[i])
		}
	}
}

// difftastic prints no headers, only a new-side line-number gutter, so a hunk
// resolves to the first row reaching its new start. Both fixtures are verbatim
// difftastic output (ANSI stripped) for a change on new-side line 8, and both
// span the 9→10 digit-width boundary — the case that distinguishes the two
// gutter anchorings.
func TestHunkOffsets_DifftasticGutter(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  int // the row displaying new-side line 8
	}{
		{
			// Inline: the new-side number is left-aligned at a fixed start column,
			// so "8" and "10" end at different columns.
			name: "inline",
			lines: []string{
				"big.go --- 1/2 --- Go",
				"4        return 1",
				"5    }",
				"6    ",
				"7    func fn02() int {",
				"8        return 2",
				"   8     return 222",
				"   9 }",
				"   10 ",
				"   11 func fn03() int {",
			},
			want: 6,
		},
		{
			// Side-by-side: the new-side number is right-aligned to a fixed end
			// column, so "8" and "10" start at different columns.
			name: "side-by-side",
			lines: []string{
				"big.go --- 1/2 --- Go",
				" 5 }                          5 }",
				" 6                            6 ",
				" 7 func fn02() int {          7 func fn02() int {",
				" 8     return 2               8     return 222",
				" 9 }                          9 }",
				"10                           10 ",
				"11 func fn03() int {         11 func fn03() int {",
			},
			want: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HunkOffsets(tt.lines, []diff.Hunk{{Header: "@@ -8,3 +8,3 @@", NewStart: 8}})
			if got[0] != tt.want {
				t.Errorf("offset = %d (%q), want %d (%q)", got[0], tt.lines[got[0]], tt.want, tt.lines[tt.want])
			}
		})
	}
}

// Every hunk keeps a position even when the render places none of them, so the
// cursor still steps through content carrying neither headers nor a gutter.
func TestHunkOffsets_UnresolvableStayOrdered(t *testing.T) {
	lines := []string{"binary files differ", "", ""}
	hunks := []diff.Hunk{{NewStart: 3}, {NewStart: 9}, {NewStart: 20}}
	got := HunkOffsets(lines, hunks)
	if len(got) != len(hunks) {
		t.Fatalf("got %d offsets, want %d", len(got), len(hunks))
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Errorf("offsets not strictly increasing: %v", got)
		}
	}
}

// TestFindGutter covers the parse of difftastic's new-side line gutter in both
// layouts. It must read the NEW side, never the old one, so an insertion that
// shifts the two apart still resolves to the right row.
func TestFindGutter(t *testing.T) {
	inline := []string{
		"167    ",                       // old-side context, margin gutter
		"168   test \"excludes hold\"",  // old-side, removed
		"   168   test \"covers hold\"", // new-side
		"   169     loan = create_loan", // new-side
		"      x := 9999",               // content number, not a gutter
	}
	g := findGutter(inline)
	if got := g.lineNum(inline[2]); got != 168 {
		t.Errorf("inline new line = %d, want 168", got)
	}
	if got := g.lineNum(inline[1]); got != -1 {
		t.Errorf("old-side row should have no new line, got %d", got)
	}

	// Side-by-side: old number at the margin, new number in a right gutter that
	// difftastic right-aligns to a fixed end column. Build rows with a constant
	// left field so the new gutter lands consistently; a removed row has "..."
	// (no new side) and one new number is 2-digit to exercise right-alignment.
	rows := []struct{ oldNum, oldTxt, newNum, newTxt string }{
		{"165", "assert_equal 900.0, x", "165", "assert_equal 900.0, x"},
		{"168", `test "excludes hold"`, "168", `test "covers hold"`},
		{"175", "loan.update!(:dont_repay)", "", ""}, // removed: no new side
		{"177", "end", "99", "end"},                  // right-aligned 2-digit
	}
	var sbs []string
	for _, r := range rows {
		right := "..."
		if r.newNum != "" {
			right = fmt.Sprintf("%3s  %s", r.newNum, r.newTxt) // 3-wide right-aligned gutter
		}
		sbs = append(sbs, fmt.Sprintf("%-3s  %-40s%s", r.oldNum, r.oldTxt, right))
	}
	g = findGutter(sbs)
	if got := g.lineNum(sbs[1]); got != 168 {
		t.Errorf("sbs new line = %d, want 168", got)
	}
	if got := g.lineNum(sbs[2]); got != -1 {
		t.Errorf("sbs removed row should have no new line, got %d", got)
	}
	if got := g.lineNum(sbs[3]); got != 99 {
		t.Errorf("sbs right-aligned 2-digit new line = %d, want 99", got)
	}

	// Content that carries no line numbers at all (the git engine's output) must
	// report no gutter, so callers fall back rather than reading a stray number.
	if findGutter([]string{"@@ -1,7 +1,7 @@", "package main", "-\treturn 1"}).ok {
		t.Error("a render without line numbers should have no gutter")
	}
}

func TestSectionRange(t *testing.T) {
	sections := []PreviewSection{{Offset: 0, Label: "a.go"}, {Offset: 10, Label: "b.go"}}
	tests := []struct {
		name       string
		sections   []PreviewSection
		label      string
		total      int
		start, end int
	}{
		{"first of several", sections, "a.go", 25, 0, 10},
		{"last runs to the end", sections, "b.go", 25, 10, 25},
		// An unmatched label must not fall back to the whole preview: resolving a
		// line number against another file's rows would land on the wrong code.
		{"unmatched is empty", sections, "c.go", 25, 0, 0},
		// A preview with no banner at all is one unlabelled span.
		{"unsectioned spans everything", nil, "a.go", 25, 0, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := SectionRange(tt.sections, tt.label, tt.total)
			if start != tt.start || end != tt.end {
				t.Errorf("SectionRange(%q) = (%d, %d), want (%d, %d)", tt.label, start, end, tt.start, tt.end)
			}
		})
	}
}
