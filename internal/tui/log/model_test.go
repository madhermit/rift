package logui

import (
	"testing"

	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/tui"
)

func TestCommitHeader(t *testing.T) {
	tests := []struct {
		name   string
		commit git.CommitInfo
		files  []git.ChangedFile
		want   string
	}{
		{
			name: "subject only no files",
			commit: git.CommitInfo{
				Hash: "abc1234", Author: "Alice", Date: "2026-01-15 14:30",
				Message: "Fix the thing",
			},
			want: "commit abc1234\nAuthor: Alice\nDate:   2026-01-15 14:30\n\n    Fix the thing\n\n─────────────────────\n\n",
		},
		{
			name: "subject and body",
			commit: git.CommitInfo{
				Hash: "def5678", Author: "Bob", Date: "2026-02-01 09:00",
				Message: "Add feature", Body: "This adds a new feature\nthat does stuff",
			},
			want: "commit def5678\nAuthor: Bob\nDate:   2026-02-01 09:00\n\n    Add feature\n\n    This adds a new feature\n    that does stuff\n\n─────────────────────\n\n",
		},
		{
			name: "with changed files",
			commit: git.CommitInfo{
				Hash: "abc1234", Author: "Alice", Date: "2026-01-15 14:30",
				Message: "Update code",
			},
			files: []git.ChangedFile{
				{Path: "main.go", Status: "Modified"},
				{Path: "README.md", Status: "Added"},
			},
			want: "commit abc1234\nAuthor: Alice\nDate:   2026-01-15 14:30\n\n    Update code\n\n" +
				"  M " + tui.FileIcon("main.go") + " main.go\n" +
				"  A " + tui.FileIcon("README.md") + " README.md\n" +
				"\n─────────────────────\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitHeader(tt.commit, tt.files, false, 0)
			if got != tt.want {
				t.Errorf("commitHeader() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
