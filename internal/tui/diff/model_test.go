package diffui

import (
	"context"
	"testing"

	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
)

type stubEngine struct{}

func (stubEngine) Name() string { return "difftastic" }
func (stubEngine) Diff(context.Context, string, diff.File, diff.DiffOpts) (string, error) {
	return "", nil
}
func (stubEngine) DiffHunks(context.Context, []diff.Hunk, string, string, bool, int) []string {
	return nil
}

// TestDisplayFilesEmpty covers the empty-changeset case: no synthetic "All
// changes" row is shown over zero files, so the empty status becomes reachable.
func TestDisplayFilesEmpty(t *testing.T) {
	// A commit diff (target set) skips reviewed marks, so it needs no repo.
	m := New(nil, stubEngine{}, nil, false, "base", "target", false, nil, false)
	if got := m.displayFiles(); got != nil {
		t.Errorf("empty changeset should display no rows, got %v", got)
	}
	// With files present, the synthetic All row leads the list.
	m = New(nil, stubEngine{}, []git.ChangedFile{{Path: "a.go"}}, false, "base", "target", false, nil, false)
	got := m.displayFiles()
	if len(got) != 2 || got[0].Path != "" || got[0].Status != "All" {
		t.Errorf("non-empty changeset should prepend the All row, got %v", got)
	}
}

// TestPreviewFiles covers the file list a selection previews: one file, or every
// changed file for the synthetic "All changes" entry.
func TestPreviewFiles(t *testing.T) {
	visible := []git.ChangedFile{{Path: ""}, {Path: "a"}, {Path: "b"}}
	if got := previewFiles(git.ChangedFile{Path: "a"}, visible); len(got) != 1 || got[0].Path != "a" {
		t.Errorf("single file: got %v", got)
	}
	if got := previewFiles(git.ChangedFile{Path: ""}, visible); len(got) != 2 || got[0].Path != "a" || got[1].Path != "b" {
		t.Errorf("all changes: got %v (should skip the empty All entry)", got)
	}
}

// TestNextUnreviewed covers the r auto-advance target: the next visible
// unreviewed file after the current one, wrapping, skipping the All row.
func TestNextUnreviewed(t *testing.T) {
	items := []git.ChangedFile{{Path: ""}, {Path: "a"}, {Path: "b"}, {Path: "c"}}
	reviewed := func(m map[string]bool) func(string) bool {
		return func(p string) bool { return m[p] }
	}

	if next := nextUnreviewed(items, "a", reviewed(map[string]bool{"a": true})); next != "b" {
		t.Errorf("advance from a: got %q, want b", next)
	}
	// b reviewed too: skip to c.
	if next := nextUnreviewed(items, "a", reviewed(map[string]bool{"a": true, "b": true})); next != "c" {
		t.Errorf("skip reviewed: got %q, want c", next)
	}
	// Wrap around past the All row.
	if next := nextUnreviewed(items, "c", reviewed(map[string]bool{"b": true, "c": true})); next != "a" {
		t.Errorf("wrap: got %q, want a", next)
	}
	// Everything reviewed: no target.
	if next := nextUnreviewed(items, "c", reviewed(map[string]bool{"a": true, "b": true, "c": true})); next != "" {
		t.Errorf("all reviewed should report no next target, got %q", next)
	}
}

// The hunk readout and {/} both ask the same question of a scroll position:
// which hunk is the reader inside? It's the last one starting at or above the
// top row — never the nearest, so scrolling into a hunk's body keeps reporting
// that hunk rather than jumping ahead to the next one. Hunks are numbered from
// 1 so that "none" is 0, and a freshly reset cursor needs no sentinel.
func TestHunkAt(t *testing.T) {
	spots := []hunkSpot{{line: 3}, {line: 20}, {line: 44}}
	tests := []struct {
		name string
		y    int
		want int
	}{
		{"above the first hunk (a file header)", 0, 0},
		{"exactly at a hunk's start", 3, 1},
		{"inside a hunk's body", 19, 1},
		{"at the next hunk", 20, 2},
		{"past the last hunk's start", 99, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hunkAt(spots, tt.y); got != tt.want {
				t.Errorf("hunkAt(%v, %d) = %d, want %d", spots, tt.y, got, tt.want)
			}
		})
	}
	if got := hunkAt(nil, 5); got != 0 {
		t.Errorf("no hunks should report none, got %d", got)
	}
}
