package lensui

import (
	"testing"

	"github.com/madhermit/rift/internal/git"
)

func TestFingerprint(t *testing.T) {
	files := []git.ChangedFile{
		{Path: "a.go", Status: "Modified", Added: 3, Deleted: 1},
		{Path: "b.go", Status: "Untracked", Added: 10},
	}
	hashes := map[string]string{"a.go": "h1", "b.go": "h2"}
	base := fingerprint(files, hashes)

	if fingerprint(files, hashes) != base {
		t.Error("fingerprint not stable for identical input")
	}

	tests := []struct {
		name   string
		files  []git.ChangedFile
		hashes map[string]string
	}{
		{
			// An edit that leaves the stat line unchanged must still register.
			name:   "content hash changed",
			files:  files,
			hashes: map[string]string{"a.go": "h1-edited", "b.go": "h2"},
		},
		{
			name:   "stat changed",
			files:  []git.ChangedFile{{Path: "a.go", Status: "Modified", Added: 4, Deleted: 1}, files[1]},
			hashes: hashes,
		},
		{
			name:   "file added",
			files:  append(append([]git.ChangedFile{}, files...), git.ChangedFile{Path: "c.go", Status: "Added"}),
			hashes: hashes,
		},
		{
			name:   "file removed",
			files:  files[:1],
			hashes: hashes,
		},
		{
			name:   "empty set",
			files:  nil,
			hashes: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if fingerprint(tt.files, tt.hashes) == base {
				t.Error("fingerprint did not change")
			}
		})
	}
}
