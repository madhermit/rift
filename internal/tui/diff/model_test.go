package diffui

import (
	"context"
	"testing"

	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
)

type stubEngine struct{}

func (stubEngine) Name() string { return "difftastic" }
func (stubEngine) Diff(context.Context, string, string, diff.DiffOpts) (string, error) {
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
	if got := previewFiles(git.ChangedFile{Path: "a"}, visible); len(got) != 1 || got[0] != "a" {
		t.Errorf("single file: got %v", got)
	}
	if got := previewFiles(git.ChangedFile{Path: ""}, visible); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("all changes: got %v (should skip the empty All entry)", got)
	}
}
