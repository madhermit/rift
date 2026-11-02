package review

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/madhermit/rift/internal/git"
)

// Where the marks live inside the git dir — per worktree, not committed (they
// sit under .git, which git itself never tracks). Hunk marks get their own file
// rather than a widened reviewed.json: that file's on-disk shape is a bare
// path→hash map, and changing it would make every marks file already out there
// unparseable — which is to say quietly moved aside and lost.
const (
	reviewedFile = "rift/reviewed.json"
	hunksFile    = "rift/hunks.json"
)

// statePath resolves a state file inside the repo's git dir, or "" when it can't
// be located — marks then live in memory for the session rather than failing.
func statePath(repo *git.Repo, name string) string {
	p, err := repo.GitPath(name)
	if err != nil {
		return ""
	}
	return p
}

// readMarks decodes a JSON state file. ok reports whether it yielded usable
// content: false for a missing file, and false for an unparseable one — which is
// additionally moved aside to <path>.corrupt, so a truncated write leaves the
// user's marks recoverable instead of silently dropping them. A literal `null`
// parses cleanly into a nil map, which callers reject rather than adopt — a nil
// map reads fine but panics on the next mark.
func readMarks[T any](path string) (marks T, ok bool) {
	if path == "" {
		return marks, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return marks, false // no marks yet
	}
	if err := json.Unmarshal(data, &marks); err != nil {
		var zero T
		_ = os.Rename(path, path+".corrupt")
		return zero, false
	}
	return marks, true
}

// saveMarks persists marks via a temp file + atomic rename, so an interrupted
// write or a racing pane never leaves a half-written (corrupt) file at the
// destination. Best-effort: a failure leaves the in-memory marks intact for this
// session.
func saveMarks(path string, marks any) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(marks, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".marks-*")
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
	_ = os.Rename(tmpName, path)
}
