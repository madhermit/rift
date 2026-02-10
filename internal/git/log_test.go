package git

import (
	"testing"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

func TestLog(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}

	for _, msg := range []string{"second commit", "third commit"} {
		writeFile(t, repo.root, "file.txt", msg)
		if _, err := wt.Add("file.txt"); err != nil {
			t.Fatalf("git add: %v", err)
		}
		testCommit(t, wt, msg)
	}

	t.Run("returns all commits in order", func(t *testing.T) {
		commits, err := repo.Log("HEAD", 0, nil)
		if err != nil {
			t.Fatalf("Log() error = %v", err)
		}
		if len(commits) != 3 {
			t.Fatalf("expected 3 commits, got %d", len(commits))
		}
		if commits[0].Message != "third commit" {
			t.Errorf("commits[0].Message = %q, want %q", commits[0].Message, "third commit")
		}
		if commits[2].Message != "initial commit" {
			t.Errorf("commits[2].Message = %q, want %q", commits[2].Message, "initial commit")
		}
	})

	t.Run("maxCount limits results", func(t *testing.T) {
		commits, err := repo.Log("HEAD", 2, nil)
		if err != nil {
			t.Fatalf("Log() error = %v", err)
		}
		if len(commits) != 2 {
			t.Fatalf("expected 2 commits, got %d", len(commits))
		}
	})

	t.Run("hash is 7 chars", func(t *testing.T) {
		commits, err := repo.Log("HEAD", 1, nil)
		if err != nil {
			t.Fatalf("Log() error = %v", err)
		}
		if len(commits[0].Hash) != 7 {
			t.Errorf("hash length = %d, want 7", len(commits[0].Hash))
		}
	})
}

func TestLog_Body(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}

	writeFile(t, repo.root, "file.txt", "content")
	if _, err := wt.Add("file.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	testCommit(t, wt, "subject line\n\nBody paragraph one.\nBody paragraph two.")

	commits, err := repo.Log("HEAD", 1, nil)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	if commits[0].Message != "subject line" {
		t.Errorf("Message = %q, want %q", commits[0].Message, "subject line")
	}
	if commits[0].Body != "Body paragraph one.\nBody paragraph two." {
		t.Errorf("Body = %q, want %q", commits[0].Body, "Body paragraph one.\nBody paragraph two.")
	}
}

func TestLog_NoBody(t *testing.T) {
	repo := setupTestRepo(t)

	commits, err := repo.Log("HEAD", 1, nil)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	if commits[0].Body != "" {
		t.Errorf("Body = %q, want empty", commits[0].Body)
	}
}

func TestLog_BadRef(t *testing.T) {
	repo := setupTestRepo(t)

	_, err := repo.Log("nonexistent-ref", 0, nil)
	if err == nil {
		t.Fatal("expected error for bad ref, got nil")
	}
}

func TestLogAll(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}

	// Second commit on main
	writeFile(t, repo.root, "main.txt", "main content")
	if _, err := wt.Add("main.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	testCommit(t, wt, "main commit")

	// Branch off and add a unique commit
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

	commits, err := repo.LogAll(0, nil)
	if err != nil {
		t.Fatalf("LogAll() error = %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("expected 3 deduplicated commits, got %d", len(commits))
	}

	seen := map[string]bool{}
	for _, c := range commits {
		if seen[c.Hash] {
			t.Errorf("duplicate hash: %s", c.Hash)
		}
		seen[c.Hash] = true
	}
}

func TestLogAll_MaxCount(t *testing.T) {
	repo := setupTestRepo(t)

	commits, err := repo.LogAll(1, nil)
	if err != nil {
		t.Fatalf("LogAll() error = %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit with maxCount=1, got %d", len(commits))
	}
}
