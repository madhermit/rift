package review

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/madhermit/rift/internal/git"
)

// reviewedFile is where the marks live inside the git dir — per worktree, not
// committed (it's under .git, which git itself never tracks).
const reviewedFile = "rift/reviewed.json"

// deletedHash is the stable content identity for a deleted file: it has no blob
// to hash, but should still be reviewable (and tick out of the list), so it gets
// this sentinel. A mark resets if the path comes back with real content.
const deletedHash = "deleted"

// ContentHashes returns each changed file's current content identity — its
// working-tree blob hash, or the deletion sentinel — keyed by path. Reviewed
// marks compare against these so a mark resets when the content changes.
func ContentHashes(repo *git.Repo, files []git.ChangedFile) map[string]string {
	hashes := repo.BlobHashes(git.Paths(files))
	for _, f := range files {
		if f.Path != "" && f.Status == "Deleted" {
			hashes[f.Path] = deletedHash
		}
	}
	return hashes
}

// Reviewed tracks which working-tree files the user has marked reviewed, keyed by
// the blob hash of the content at the moment they marked it. A mark therefore
// resets the instant the file changes (the current hash no longer matches), and
// re-applies if the file is reverted to a state that was already reviewed.
type Reviewed struct {
	path  string            // state file; "" if it couldn't be resolved (marks are then in-memory only)
	marks map[string]string // file path → reviewed blob hash
}

// LoadReviewed reads the persisted marks for the repo, or an empty set if there
// are none / the file is unreadable. A corrupt (unparseable) file is moved aside
// to <file>.corrupt before starting empty, so a truncated write preserves the
// user's marks for recovery rather than silently dropping them.
func LoadReviewed(repo *git.Repo) *Reviewed {
	r := &Reviewed{marks: map[string]string{}}
	p, err := repo.GitPath(reviewedFile)
	if err != nil {
		return r
	}
	r.path = p
	data, err := os.ReadFile(p)
	if err != nil {
		return r // no marks yet
	}
	if err := json.Unmarshal(data, &r.marks); err != nil {
		r.marks = map[string]string{}
		_ = os.Rename(p, p+".corrupt")
	}
	return r
}

// IsReviewed reports whether path is marked reviewed at its current blob hash.
func (r *Reviewed) IsReviewed(path, currentHash string) bool {
	return currentHash != "" && r.marks[path] == currentHash
}

// Toggle flips path's reviewed mark at the given hash and persists, returning the
// new state. A file with no content hash (e.g. a deletion) can't be marked. It
// re-reads the on-disk marks first so a concurrent rift pane's marks are merged
// in rather than clobbered by this write.
func (r *Reviewed) Toggle(path, currentHash string) bool {
	r.reload()
	if r.IsReviewed(path, currentHash) {
		delete(r.marks, path)
	} else if currentHash != "" {
		r.marks[path] = currentHash
	}
	r.save()
	return r.IsReviewed(path, currentHash)
}

// reload merges the current on-disk marks into memory before a mutation, so a
// concurrent pane's additions survive this instance's next save. A missing or
// unparseable file leaves the in-memory marks untouched (a save will overwrite it
// atomically anyway).
func (r *Reviewed) reload() {
	if r.path == "" {
		return
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		return
	}
	var disk map[string]string
	if err := json.Unmarshal(data, &disk); err != nil {
		return
	}
	r.marks = disk
}

// Unreviewed returns the files whose current content isn't marked reviewed,
// given their content hashes (see ContentHashes).
func (r *Reviewed) Unreviewed(files []git.ChangedFile, hashes map[string]string) []git.ChangedFile {
	out := make([]git.ChangedFile, 0, len(files))
	for _, f := range files {
		if !r.IsReviewed(f.Path, hashes[f.Path]) {
			out = append(out, f)
		}
	}
	return out
}

// Count returns how many of the given path→hash pairs are currently reviewed.
func (r *Reviewed) Count(hashes map[string]string) int {
	n := 0
	for path, h := range hashes {
		if r.IsReviewed(path, h) {
			n++
		}
	}
	return n
}

// save persists the marks via a temp file + atomic rename, so an interrupted
// write or a racing pane never leaves a half-written (corrupt) marks file at the
// destination. Best-effort: a persistence failure leaves the in-memory marks
// intact for this session.
func (r *Reviewed) save() {
	if r.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(r.marks, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".reviewed-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed away
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Chmod(tmpName, 0o644)
	_ = os.Rename(tmpName, r.path)
}
