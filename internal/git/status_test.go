package git

import "testing"

// TestParseStatusZ covers parsing of `git status --porcelain -z --no-renames`:
// the two status columns, untracked files, a both-staged-and-modified file, and
// a rename (which --no-renames splits into a plain Deleted + Added pair — no
// trailing original-path field).
func TestParseStatusZ(t *testing.T) {
	in := " M a.go\x00A  b.go\x00?? junk\x00MM both\x00D  old.go\x00A  new.go\x00"
	got := parseStatusZ(in)

	want := []StatusFile{
		{Path: "a.go", StagingStatus: "", WorktreeStatus: "Modified"},
		{Path: "b.go", StagingStatus: "Added", WorktreeStatus: ""},
		{Path: "junk", StagingStatus: "Untracked", WorktreeStatus: "Untracked"},
		{Path: "both", StagingStatus: "Modified", WorktreeStatus: "Modified"},
		{Path: "old.go", StagingStatus: "Deleted", WorktreeStatus: ""},
		{Path: "new.go", StagingStatus: "Added", WorktreeStatus: ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("file %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestStatusWord(t *testing.T) {
	cases := map[byte]string{
		'M': "Modified", 'A': "Added", 'D': "Deleted", 'R': "Renamed",
		'C': "Copied", '?': "Untracked", ' ': "", 0: "", 'U': "U",
	}
	for code, want := range cases {
		if got := statusWord(code); got != want {
			t.Errorf("statusWord(%q) = %q, want %q", code, got, want)
		}
	}
}
