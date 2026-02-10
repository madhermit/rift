package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

var testSigTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// setupTestRepo creates a temporary git repo with an initial commit and returns
// the Repo handle. The repo is cleaned up when the test finishes.
func setupTestRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()

	r, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}

	writeFile(t, dir, "README.md", "# test repo\n")
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	testCommit(t, wt, "initial commit")

	return &Repo{repo: r, root: dir}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func testCommit(t *testing.T, wt *gogit.Worktree, msg string) plumbing.Hash {
	t.Helper()
	testSigTime = testSigTime.Add(time.Second)
	h, err := wt.Commit(msg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
			When:  testSigTime,
		},
	})
	if err != nil {
		t.Fatalf("commit %q: %v", msg, err)
	}
	return h
}
