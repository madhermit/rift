package review

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madhermit/rift/internal/diff"
)

func TestHunkKey(t *testing.T) {
	body := []string{" ctx", "-old", "+new"}
	tests := []struct {
		name string
		a, b diff.Hunk
		same bool
	}{
		{
			name: "same body under a shifted header",
			a:    diff.Hunk{Header: "@@ -1,3 +1,3 @@", Lines: body},
			b:    diff.Hunk{Header: "@@ -80,3 +91,3 @@ func f()", Lines: body},
			same: true,
		},
		{
			name: "different body",
			a:    diff.Hunk{Header: "@@ -1,3 +1,3 @@", Lines: body},
			b:    diff.Hunk{Header: "@@ -1,3 +1,3 @@", Lines: []string{" ctx", "-old", "+other"}},
		},
		{
			name: "a line moved between hunks is not the same hunk",
			a:    diff.Hunk{Lines: []string{"+a", "+b"}},
			b:    diff.Hunk{Lines: []string{"+b", "+a"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HunkKey(tt.a) == HunkKey(tt.b); got != tt.same {
				t.Errorf("keys equal = %v, want %v", got, tt.same)
			}
		})
	}
}

func TestHunksToggle(t *testing.T) {
	repo := openTempRepo(t)
	h := LoadHunks(repo)

	if h.IsReviewed("f.go", "k1") {
		t.Fatal("nothing should be marked initially")
	}
	if !h.Toggle("f.go", "k1") {
		t.Fatal("toggle should mark the hunk")
	}
	if h.IsReviewed("f.go", "k2") {
		t.Error("marking k1 must not mark another hunk")
	}
	if h.IsReviewed("other.go", "k1") {
		t.Error("marks are per file: the same key elsewhere is unmarked")
	}
	if !LoadHunks(repo).IsReviewed("f.go", "k1") {
		t.Error("mark should persist across loads")
	}
	if h.Toggle("f.go", "k1") {
		t.Error("second toggle should unmark")
	}
	if LoadHunks(repo).IsReviewed("f.go", "k1") {
		t.Error("unmark should persist")
	}
}

func TestHunksReviewed(t *testing.T) {
	repo := openTempRepo(t)
	h := LoadHunks(repo)
	keys := []string{"k1", "k2", "k3"}

	if n, all := h.Reviewed("f.go", keys); n != 0 || all {
		t.Errorf("fresh store: got (%d, %v), want (0, false)", n, all)
	}
	if n, all := h.Reviewed("f.go", nil); n != 0 || all {
		t.Errorf("a file with no hunks is never complete: got (%d, %v)", n, all)
	}

	h.Toggle("f.go", "k2")
	if n, all := h.Reviewed("f.go", keys); n != 1 || all {
		t.Errorf("one of three marked: got (%d, %v), want (1, false)", n, all)
	}
	h.Toggle("f.go", "k1")
	h.Toggle("f.go", "k3")
	if n, all := h.Reviewed("f.go", keys); n != 3 || !all {
		t.Errorf("all marked: got (%d, %v), want (3, true)", n, all)
	}

	// A mark whose hunk is no longer in the diff (the other staged side, or a
	// rewrite) is kept, not pruned — but it doesn't count toward this scope.
	if n, all := h.Reviewed("f.go", []string{"k1"}); n != 1 || !all {
		t.Errorf("narrowed scope: got (%d, %v), want (1, true)", n, all)
	}
	if !LoadHunks(repo).IsReviewed("f.go", "k3") {
		t.Error("a key outside the scope just queried must survive")
	}
}

// Two panes marking hunks in different files must not clobber each other: b
// loaded before a persisted, so without the reload-before-save merge b's write
// would drop a's mark.
func TestHunksConcurrentMerge(t *testing.T) {
	repo := openTempRepo(t)
	a, b := LoadHunks(repo), LoadHunks(repo)

	a.Toggle("file1.go", "k1")
	b.Toggle("file2.go", "k2")

	got := LoadHunks(repo)
	if !got.IsReviewed("file1.go", "k1") {
		t.Error("file1 mark lost — a concurrent save clobbered it")
	}
	if !got.IsReviewed("file2.go", "k2") {
		t.Error("file2 mark missing")
	}
}

func TestHunksCorruptBackup(t *testing.T) {
	repo := openTempRepo(t)
	p, err := repo.GitPath(hunksFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if LoadHunks(repo).IsReviewed("anything", "k") {
		t.Error("a corrupt marks file should load as empty")
	}
	if _, err := os.Stat(p + ".corrupt"); err != nil {
		t.Errorf("corrupt file should be backed up to %s.corrupt: %v", p, err)
	}
}

// A JSON null decodes into a nil map, which reads fine but panics on the next
// mark — the loader must reject it rather than adopt it.
func TestHunksNullFile(t *testing.T) {
	repo := openTempRepo(t)
	p, _ := repo.GitPath(hunksFile)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("null"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !LoadHunks(repo).Toggle("f.go", "k1") {
		t.Error("a null marks file should load as empty and stay writable")
	}
}
