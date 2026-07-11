package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitPath resolves a path inside the git directory (e.g. "rift/reviewed.json"),
// honoring linked-worktree layouts where the gitdir lives elsewhere. It does not
// create anything — callers MkdirAll the parent before writing.
func (r *Repo) GitPath(rel string) (string, error) {
	out, err := r.runGit("rev-parse", "--git-path", rel)
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(out)
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.root, p)
	}
	return p, nil
}

// StagedBlobHashes returns the blob hash of each path's content in the index,
// in one subprocess (`git ls-files --stage`). Paths absent from the index are
// omitted. This is the staged-side counterpart of BlobHashes: the index
// already stores blob OIDs, so nothing is hashed.
func (r *Repo) StagedBlobHashes(paths []string) map[string]string {
	hashes := map[string]string{}
	if len(paths) == 0 {
		return hashes
	}
	args := append([]string{"ls-files", "--stage", "-z", "--"}, paths...)
	out, err := r.runGit(args...)
	if err != nil {
		return hashes
	}
	// Records: "<mode> <oid> <stage>\t<path>", NUL-terminated.
	for _, rec := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) == 3 {
			hashes[path] = fields[1]
		}
	}
	return hashes
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
