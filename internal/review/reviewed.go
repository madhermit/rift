package review

import "github.com/madhermit/rift/internal/git"

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
// are none / the file is unreadable.
func LoadReviewed(repo *git.Repo) *Reviewed {
	r := &Reviewed{marks: map[string]string{}}
	r.path = statePath(repo, reviewedFile)
	if marks, ok := readMarks[map[string]string](r.path); ok && marks != nil {
		r.marks = marks
	}
	return r
}

// IsReviewed reports whether path is marked reviewed at its current blob hash.
func (r *Reviewed) IsReviewed(path, currentHash string) bool {
	return currentHash != "" && r.marks[path] == currentHash
}

// Toggle flips path's reviewed mark at the given hash and persists, returning the
// new state.
func (r *Reviewed) Toggle(path, currentHash string) bool {
	r.reload()
	return r.write(path, currentHash, !r.IsReviewed(path, currentHash))
}

// Set drives path's mark to a known state rather than flipping it, for a caller
// that has already decided — the diff screen keeps a file's mark in step with its
// hunks this way, where a Toggle would invert the very state it just computed.
func (r *Reviewed) Set(path, currentHash string, on bool) bool {
	r.reload()
	return r.write(path, currentHash, on)
}

// write applies the mark to the already-reloaded marks and persists. A file with
// no content hash (e.g. a deletion outside the changed set) can't be marked.
func (r *Reviewed) write(path, currentHash string, on bool) bool {
	if on && currentHash != "" {
		r.marks[path] = currentHash
	} else {
		delete(r.marks, path)
	}
	r.save()
	return r.IsReviewed(path, currentHash)
}

// reload merges the current on-disk marks into memory before a mutation, so a
// concurrent pane's additions survive this instance's next save. Nothing usable
// on disk leaves the in-memory marks untouched (a save will overwrite it
// atomically anyway).
func (r *Reviewed) reload() {
	if marks, ok := readMarks[map[string]string](r.path); ok && marks != nil {
		r.marks = marks
	}
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

func (r *Reviewed) save() { saveMarks(r.path, r.marks) }
