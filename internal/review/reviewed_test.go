package review

import (
	"os"
	"os/exec"
	"testing"

	"github.com/madhermit/rift/internal/git"
)

func TestReviewed(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t.co"}, {"config", "user.name", "t"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if err := os.WriteFile(dir+"/f.go", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	os.Chdir(dir)

	repo, err := git.OpenRepo()
	if err != nil {
		t.Fatalf("open repo: %v", err)
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
