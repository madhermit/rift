package git

import (
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestListBranches(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}

	// Create a second branch with its own commit
	head, err := repo.repo.Head()
	if err != nil {
		t.Fatalf("get head: %v", err)
	}
	ref := plumbing.NewHashReference("refs/heads/feature", head.Hash())
	if err := repo.repo.Storer.SetReference(ref); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if err := wt.Checkout(&gogit.CheckoutOptions{Branch: "refs/heads/feature"}); err != nil {
		t.Fatalf("checkout feature: %v", err)
	}
	writeFile(t, repo.root, "feature.txt", "feature content")
	if _, err := wt.Add("feature.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	testCommit(t, wt, "feature commit")

	branches, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}

	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}

	// Current branch (feature) should have Current=true
	var found bool
	for _, b := range branches {
		if b.Name == "feature" {
			found = true
			if !b.Current {
				t.Error("feature branch should be current")
			}
			if b.Message != "feature commit" {
				t.Errorf("feature message = %q, want %q", b.Message, "feature commit")
			}
		}
		if b.Name == "master" && b.Current {
			t.Error("master should not be current")
		}
	}
	if !found {
		t.Error("feature branch not found in results")
	}
}

func TestListBranches_CurrentFirst(t *testing.T) {
	repo := setupTestRepo(t)

	// Create branches that sort alphabetically before "master"
	head, err := repo.repo.Head()
	if err != nil {
		t.Fatalf("get head: %v", err)
	}
	for _, name := range []string{"alpha", "beta"} {
		ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), head.Hash())
		if err := repo.repo.Storer.SetReference(ref); err != nil {
			t.Fatalf("create branch %s: %v", name, err)
		}
	}

	branches, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}

	if len(branches) != 3 {
		t.Fatalf("expected 3 branches, got %d", len(branches))
	}

	// Current branch (master) must be first
	if !branches[0].Current {
		t.Errorf("first branch should be current, got %q (current=%v)", branches[0].Name, branches[0].Current)
	}
	if branches[0].Name != "master" {
		t.Errorf("first branch name = %q, want %q", branches[0].Name, "master")
	}

	// Remaining should be alphabetical
	if branches[1].Name != "alpha" {
		t.Errorf("branches[1].Name = %q, want %q", branches[1].Name, "alpha")
	}
	if branches[2].Name != "beta" {
		t.Errorf("branches[2].Name = %q, want %q", branches[2].Name, "beta")
	}
}
