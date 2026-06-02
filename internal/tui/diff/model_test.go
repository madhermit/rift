package diffui

import (
	"testing"

	"github.com/madhermit/rift/internal/git"
)

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
