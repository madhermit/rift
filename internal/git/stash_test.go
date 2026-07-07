package git

import "testing"

// TestListStashes_Empty confirms an empty stash list is reported as no entries,
// not an error — `git stash list` exits 0 when there are no stashes.
func TestListStashes_Empty(t *testing.T) {
	repo := setupTestRepo(t)

	stashes, err := repo.ListStashes()
	if err != nil {
		t.Fatalf("ListStashes() error = %v", err)
	}
	if len(stashes) != 0 {
		t.Fatalf("expected 0 stashes, got %d: %+v", len(stashes), stashes)
	}
}
