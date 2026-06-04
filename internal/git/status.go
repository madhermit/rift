package git

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type StatusFile struct {
	Path           string `json:"path"`
	StagingStatus  string `json:"staging_status"`
	WorktreeStatus string `json:"worktree_status"`
}

func (r *Repo) StatusFiles() ([]StatusFile, error) {
	files, err := r.statusFiles()
	if err != nil {
		return nil, err
	}
	// git status -z emits records in path order; sort to make that a guarantee.
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// statusFiles lists working-tree status via shelled `git status`. go-git's
// Worktree.Status() walks into gitignored directories instead of pruning them,
// so it is pathologically slow (tens of seconds) in a repo with large ignored
// subtrees like node_modules or vendor; native git prunes at the ignore rule and
// uses its untracked cache, returning in milliseconds. Shelling also lists
// untracked files inside linked worktrees, which the old `git diff --name-status`
// fallback missed.
//
// --no-renames reports a rename as a delete + an add on the real paths, matching
// what the old go-git path produced (go-git does no rename detection) and keeping
// every record to a single "XY PATH" shape — no trailing original-path field.
func (r *Repo) statusFiles() ([]StatusFile, error) {
	cmd := exec.Command("git", "status", "--porcelain", "-z", "--no-renames", "--untracked-files=all")
	cmd.Dir = r.root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	return parseStatusZ(string(out)), nil
}

// parseStatusZ parses `git status --porcelain -z --no-renames`: each NUL-
// terminated record is "XY PATH", where X is the index status and Y the worktree
// status. With renames disabled, no record carries a trailing original path, so
// each one stands alone.
func parseStatusZ(out string) []StatusFile {
	files := []StatusFile{}
	for _, rec := range strings.Split(out, "\x00") {
		if len(rec) < 4 { // trailing empty field, or too short to hold "XY PATH"
			continue
		}
		files = append(files, StatusFile{
			Path:           rec[3:],
			StagingStatus:  statusWord(rec[0]),
			WorktreeStatus: statusWord(rec[1]),
		})
	}
	return files
}

// statusWord maps a git status code byte (porcelain status or name-status) to a
// status word. A space (or NUL) means no change on that side; an unmerged or
// unknown code passes through verbatim. Shared by parseStatusZ and nameStatusCode.
func statusWord(c byte) string {
	switch c {
	case 'M':
		return "Modified"
	case 'A':
		return "Added"
	case 'D':
		return "Deleted"
	case 'R':
		return "Renamed"
	case 'C':
		return "Copied"
	case '?':
		return "Untracked"
	case ' ', 0:
		return ""
	default:
		return string(c)
	}
}

func StatusChar(status string) string {
	switch status {
	case "Modified":
		return "M"
	case "Added":
		return "A"
	case "Deleted":
		return "D"
	case "Renamed":
		return "R"
	case "Copied":
		return "C"
	case "Untracked":
		return "?"
	default:
		return " "
	}
}
