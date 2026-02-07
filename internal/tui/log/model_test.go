package logui

import (
	"testing"

	"github.com/madhermit/flux/internal/git"
)

func TestCommitHeader(t *testing.T) {
	tests := []struct {
		name   string
		commit git.CommitInfo
		want   string
	}{
		{
			name: "subject only",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitHeader(tt.commit)
			if got != tt.want {
				t.Errorf("commitHeader() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
