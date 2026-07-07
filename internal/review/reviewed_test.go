package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/madhermit/rift/internal/git"
)

// openTempRepo inits a git repo in a temp dir, chdirs into it (restored on
// cleanup), and returns an opened Repo.
func openTempRepo(t *testing.T) *git.Repo {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t.co"}, {"config", "user.name", "t"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	repo, err := git.OpenRepo()
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	return repo
}

func TestReviewed(t *testing.T) {
	repo := openTempRepo(t)
	if err := os.WriteFile("f.go", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := LoadReviewed(repo)
	if r.IsReviewed("f.go", "h1") {
		t.Fatal("nothing should be reviewed initially")
	}
	if !r.Toggle("f.go", "h1") {
		t.Fatal("toggle should mark reviewed")
	}
	if !r.IsReviewed("f.go", "h1") {
		t.Error("f.go should be reviewed at h1")
	}
	// A changed file (different hash) is no longer reviewed.
	if r.IsReviewed("f.go", "h2") {
		t.Error("reviewed mark should not match a different hash")
	}
	// Persisted: a fresh load sees the mark.
	if reloaded := LoadReviewed(repo); !reloaded.IsReviewed("f.go", "h1") {
		t.Error("mark should persist across loads")
	}
	// Toggling again clears it.
	if r.Toggle("f.go", "h1") {
		t.Error("second toggle should unmark")
	}
	if LoadReviewed(repo).IsReviewed("f.go", "h1") {
		t.Error("unmark should persist")
	}
}

// TestReviewedConcurrentMerge covers item 10: two instances marking different
// files must not clobber each other. b loads before a persists, so without the
// reload-before-save merge, b's write would drop a's mark.
func TestReviewedConcurrentMerge(t *testing.T) {
	repo := openTempRepo(t)
	a := LoadReviewed(repo)
	b := LoadReviewed(repo)

	a.Toggle("file1.go", "h1")
	b.Toggle("file2.go", "h2")

	got := LoadReviewed(repo)
	if !got.IsReviewed("file1.go", "h1") {
		t.Error("file1 mark lost — a concurrent save clobbered it")
	}
	if !got.IsReviewed("file2.go", "h2") {
		t.Error("file2 mark missing")
	}
}

// TestReviewedCorruptBackup covers item 10: an unparseable marks file is moved
// aside to <file>.corrupt (preserving it for recovery) rather than silently reset.
func TestReviewedCorruptBackup(t *testing.T) {
	repo := openTempRepo(t)
	p, err := repo.GitPath(reviewedFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := LoadReviewed(repo)
	if r.IsReviewed("anything", "h") {
		t.Error("a corrupt marks file should load as empty")
	}
	if _, err := os.Stat(p + ".corrupt"); err != nil {
		t.Errorf("corrupt file should be backed up to %s.corrupt: %v", p, err)
	}
}
