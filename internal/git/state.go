package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitPath resolves a path inside the git directory (e.g. "rift/reviewed.json"),
// honoring linked-worktree layouts where the gitdir lives elsewhere. It does not
// create anything — callers MkdirAll the parent before writing.
func (r *Repo) GitPath(rel string) (string, error) {
	out, err := exec.Command("git", "-C", r.root, "rev-parse", "--git-path", rel).Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-path %s: %w", rel, err)
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.root, p)
	}
	return p, nil
}

// BlobHashes returns the git blob hash of each path's current working-tree
// content, in one subprocess. Paths that don't exist on disk (a deleted file, a
// directory) are omitted — there's no content to hash.
func (r *Repo) BlobHashes(paths []string) map[string]string {
	hashes := map[string]string{}
	var existing []string
	for _, p := range paths {
		if fi, err := os.Stat(filepath.Join(r.root, p)); err == nil && !fi.IsDir() {
			existing = append(existing, p)
		}
	}
	if len(existing) == 0 {
		return hashes
	}
	cmd := exec.Command("git", "hash-object", "--stdin-paths")
	cmd.Dir = r.root
	cmd.Stdin = strings.NewReader(strings.Join(existing, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		return hashes
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != len(existing) {
		return hashes // a path/output mismatch would mis-map hashes; bail safely
	}
	for i, p := range existing {
		hashes[p] = lines[i]
	}
	return hashes
}
