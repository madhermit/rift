package git

import (
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestDiffTargets(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantBase   string
		wantTarget string
		wantErr    bool
	}{
		{"zero args defaults to HEAD", []string{}, "HEAD", "", false},
		{"one arg is base", []string{"main"}, "main", "", false},
		{"two args are base and target", []string{"abc", "def"}, "abc", "def", false},
		{"three args is error", []string{"a", "b", "c"}, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, target, err := DiffTargets(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DiffTargets() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if base != tt.wantBase {
				t.Errorf("base = %q, want %q", base, tt.wantBase)
			}
			if target != tt.wantTarget {
				t.Errorf("target = %q, want %q", target, tt.wantTarget)
			}
		})
	}
}

func TestStatusCodeToString(t *testing.T) {
	tests := []struct {
		name string
		code gogit.StatusCode
		want string
	}{
		{"modified", 'M', "Modified"},
		{"added", 'A', "Added"},
		{"deleted", 'D', "Deleted"},
		{"renamed", 'R', "Renamed"},
		{"copied", 'C', "Copied"},
		{"untracked", '?', "Untracked"},
		{"space", ' ', ""},
		{"zero value", 0, ""},
		{"unknown code", 'X', "X"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusCodeToString(tt.code); got != tt.want {
				t.Errorf("statusCodeToString(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestDiffActionString(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want string
	}{
		{"added", "", "newfile.go", "Added"},
		{"deleted", "oldfile.go", "", "Deleted"},
		{"modified", "file.go", "file.go", "Modified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &object.Change{
				From: object.ChangeEntry{Name: tt.from},
				To:   object.ChangeEntry{Name: tt.to},
			}
			if got := diffActionString(c); got != tt.want {
				t.Errorf("diffActionString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single line", "hello", "hello"},
		{"multi-line", "first\nsecond\nthird", "first"},
		{"empty string", "", ""},
		{"trailing newline", "hello\n", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstLine(tt.input); got != tt.want {
				t.Errorf("firstLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestChangedFiles_Unstaged(t *testing.T) {
	repo := setupTestRepo(t)

	writeFile(t, repo.root, "README.md", "# modified\n")

	files, err := repo.ChangedFiles(false)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 changed file, got %d: %v", len(files), files)
	}
	if files[0].Path != "README.md" || files[0].Status != "Modified" {
		t.Errorf("got {%q, %q}, want {\"README.md\", \"Modified\"}", files[0].Path, files[0].Status)
	}
}

func TestChangedFiles_Staged(t *testing.T) {
	repo := setupTestRepo(t)

	writeFile(t, repo.root, "new.txt", "new content\n")
	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	if _, err := wt.Add("new.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	files, err := repo.ChangedFiles(true)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 staged file, got %d: %v", len(files), files)
	}
	if files[0].Path != "new.txt" || files[0].Status != "Added" {
		t.Errorf("got {%q, %q}, want {\"new.txt\", \"Added\"}", files[0].Path, files[0].Status)
	}
}

func TestDiffBetweenCommits(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}

	head, err := repo.repo.Head()
	if err != nil {
		t.Fatalf("get head: %v", err)
	}
	baseHash := head.Hash().String()

	writeFile(t, repo.root, "added.go", "package main\n")
	writeFile(t, repo.root, "README.md", "# changed\n")
	if _, err := wt.Add("added.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	targetHash := testCommit(t, wt, "second commit")

	files, err := repo.DiffBetweenCommits(baseHash, targetHash.String())
	if err != nil {
		t.Fatalf("DiffBetweenCommits() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 changed files, got %d: %v", len(files), files)
	}

	byPath := map[string]string{}
	for _, f := range files {
		byPath[f.Path] = f.Status
	}
	if byPath["added.go"] != "Added" {
		t.Errorf("added.go status = %q, want %q", byPath["added.go"], "Added")
	}
	if byPath["README.md"] != "Modified" {
		t.Errorf("README.md status = %q, want %q", byPath["README.md"], "Modified")
	}
}
