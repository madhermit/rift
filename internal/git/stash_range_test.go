package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStashRange(t *testing.T) {
	tests := []struct {
		name      string
		stashArgs []string
		want      map[string]string // path → status
	}{
		{
			name:      "tracked changes only",
			stashArgs: []string{"stash", "push", "-q"},
			want:      map[string]string{"a.txt": "Modified"},
		},
		{
			name:      "includes untracked files from stash -u",
			stashArgs: []string{"stash", "push", "-q", "-u"},
			want:      map[string]string{"a.txt": "Modified", "new.txt": "Added"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@b", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@b")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
			}
			write := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			run("init", "-q", "-b", "main")
			write("a.txt", "one\n")
			run("add", ".")
			run("commit", "-qm", "init")
			write("a.txt", "two\n")
			write("new.txt", "fresh\n")
			run(tt.stashArgs...)

			t.Chdir(dir)
			repo, err := OpenRepo()
			if err != nil {
				t.Fatal(err)
			}
			base, target := repo.StashRange(0)
			files, err := repo.DiffBetweenCommits(base, target)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, f := range files {
				got[f.Path] = f.Status
			}
			if len(got) != len(tt.want) {
				t.Fatalf("files = %v, want %v", got, tt.want)
			}
			for path, status := range tt.want {
				if got[path] != status {
					t.Errorf("%s = %q, want %q", path, got[path], status)
				}
			}
		})
	}
}
