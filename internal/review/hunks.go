package review

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
)

// HunkKey is a hunk's content identity: a hash of its body — the ± and context
// lines — with the @@ header deliberately left out. The header carries line
// numbers that shift whenever anything above the hunk changes, so including it
// would drop a mark on code that didn't change at all.
//
// This is the same anchor the planned hunk notes use (see the design doc), but
// only the anchor: a mark is destructive — rewrite the hunk and it's gone —
// while a note on rewritten code goes stale rather than disappearing, which is
// the whole point of it. Share the key, not the lifecycle.
func HunkKey(h diff.Hunk) string {
	sum := sha256.Sum256([]byte(strings.Join(h.Lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// Hunks tracks the individual hunks the user has marked reviewed. Like a file
// mark it is content-keyed and destructive: rewrite the hunk and its hash stops
// naming anything in the diff, so the code comes back unreviewed.
type Hunks struct {
	path  string              // state file; "" if it couldn't be resolved (marks are then in-memory only)
	marks map[string][]string // file path → reviewed hunk keys
	sets  map[string]map[string]bool
}

// LoadHunks reads the persisted hunk marks for the repo, or an empty set if
// there are none / the file is unreadable.
func LoadHunks(repo *git.Repo) *Hunks {
	h := &Hunks{marks: map[string][]string{}}
	h.path = statePath(repo, hunksFile)
	if marks, ok := readMarks[map[string][]string](h.path); ok && marks != nil {
		h.marks = marks
	}
	return h
}

// IsReviewed reports whether the hunk with the given key is marked in path.
func (h *Hunks) IsReviewed(path, key string) bool { return key != "" && h.set(path)[key] }

// Reviewed counts how many of a file's hunk keys are marked, and reports whether
// all of them are. A file with no keys is never "all reviewed": there is nothing
// there to have reviewed, and saying otherwise would tick off every file while
// the hunk index is still loading.
func (h *Hunks) Reviewed(path string, keys []string) (n int, all bool) {
	if len(h.marks[path]) == 0 {
		return 0, false // nothing marked here, so nothing to look up
	}
	for _, k := range keys {
		if h.IsReviewed(path, k) {
			n++
		}
	}
	return n, len(keys) > 0 && n == len(keys)
}

// Toggle flips a hunk's mark and persists, returning the new state.
//
// Marks are never pruned against the diff in view, and mustn't be: the staged
// and unstaged sides of a path cut different hunks out of the same file, so
// dropping the keys that don't appear in the current scope would silently
// destroy the other scope's marks. A key that no longer names a hunk addresses
// nothing and costs only bytes.
func (h *Hunks) Toggle(path, key string) bool {
	h.reload()
	if key == "" {
		return false
	}
	if h.IsReviewed(path, key) {
		if rest := without(h.marks[path], key); len(rest) > 0 {
			h.marks[path] = rest
		} else {
			delete(h.marks, path)
		}
	} else {
		h.marks[path] = append(h.marks[path], key)
	}
	h.sets = nil
	h.save()
	return h.IsReviewed(path, key)
}

// reload merges the on-disk marks in before a mutation (see Reviewed.reload).
func (h *Hunks) reload() {
	if marks, ok := readMarks[map[string][]string](h.path); ok && marks != nil {
		h.marks, h.sets = marks, nil
	}
}

// set is path's marks as a lookup set, memoized so the row renderer — which asks
// about every hunk of every visible file, every frame — doesn't rescan a slice
// each time. Any mutation drops the memo wholesale; the sets are small and the
// mutations are keystrokes.
func (h *Hunks) set(path string) map[string]bool {
	if s, ok := h.sets[path]; ok {
		return s
	}
	s := make(map[string]bool, len(h.marks[path]))
	for _, k := range h.marks[path] {
		s[k] = true
	}
	if h.sets == nil {
		h.sets = map[string]map[string]bool{}
	}
	h.sets[path] = s
	return s
}

func (h *Hunks) save() { saveMarks(h.path, h.marks) }

// without returns keys with key removed, into a fresh backing array — the slice
// it copies from came off a reload and may be shared with the memoized set.
func without(keys []string, key string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != key {
			out = append(out, k)
		}
	}
	return out
}
