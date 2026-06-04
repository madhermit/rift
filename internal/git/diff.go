package git

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// SortByPath orders changed files by path, for a stable listing (go-git returns
// map order). Callers that present a file list sort through this so the ordering
// rule lives in one place.
func SortByPath(files []ChangedFile) {
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
}

// EmptyTree is git's well-known empty-tree object, used as the base when
// diffing a root commit (which has no parent).
const EmptyTree = "4b825dc642cb6eb9a060e54bf899d69f82cf7207"

type ChangedFile struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

// Paths returns just the paths of the changed files.
func Paths(files []ChangedFile) []string {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return paths
}

// ChangedFiles lists the working-tree files with a staged change (staged=true)
// or an unstaged/untracked change (staged=false), derived from `git status`.
func (r *Repo) ChangedFiles(staged bool) ([]ChangedFile, error) {
	sf, err := r.statusFiles()
	if err != nil {
		return nil, err
	}
	var files []ChangedFile
	for _, f := range sf {
		var code string
		if staged {
			// An untracked file has no staged change; everything else with an
			// index status (M/A/D/R/C) counts as staged.
			if f.StagingStatus == "" || f.StagingStatus == "Untracked" {
				continue
			}
			code = f.StagingStatus
		} else {
			if f.WorktreeStatus == "" {
				continue
			}
			code = f.WorktreeStatus
		}
		files = append(files, ChangedFile{Path: f.Path, Status: code})
	}
	r.addStats(files, staged)
	return files, nil
}

// addStats fills in per-file added/deleted line counts from git diff --numstat.
// Best-effort: stats are a display nicety, so failures leave counts at zero.
func (r *Repo) addStats(files []ChangedFile, staged bool) {
	if len(files) == 0 {
		return // nothing to annotate; skip the git subprocess
	}
	args := []string{"diff", "--numstat"}
	if staged {
		args = append(args, "--staged")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	out, err := cmd.Output()
	if err != nil {
		return
	}
	stats := parseNumstat(string(out))
	for i := range files {
		if s, ok := stats[files[i].Path]; ok {
			files[i].Added, files[i].Deleted = s[0], s[1]
		}
	}
}

// parseNumstat parses "added\tdeleted\tpath" lines. Binary files report "-".
func parseNumstat(out string) map[string][2]int {
	stats := make(map[string][2]int)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		added, _ := strconv.Atoi(parts[0])   // "-" (binary) → 0
		deleted, _ := strconv.Atoi(parts[1]) // "-" (binary) → 0
		stats[parts[2]] = [2]int{added, deleted}
	}
	return stats
}

func parseNameStatus(out string) []ChangedFile {
	var files []ChangedFile
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		// A rename/copy row has three tab fields ("R100\told\tnew"); the new path
		// is last. A plain change has two ("M\tpath"). Take the final field either
		// way so a renamed path isn't returned as the literal "old\tnew".
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		files = append(files, ChangedFile{
			Path:   parts[len(parts)-1],
			Status: nameStatusCode(parts[0]),
		})
	}
	return files
}

// nameStatusCode maps a `git diff --name-status` code to a status word. The code
// is a single letter, except renames/copies carry a similarity score (R100,
// C75), so the leading byte is the status — which statusWord already maps.
func nameStatusCode(code string) string {
	if code == "" {
		return ""
	}
	return statusWord(code[0])
}

func DiffTargets(args []string) (base, target string, err error) {
	switch len(args) {
	case 0:
		// No ref: a working-tree diff. Leave the base empty so the staged flag
		// alone picks the comparison — unstaged is worktree-vs-index, staged is
		// index-vs-HEAD. Defaulting to "HEAD" would diff the unstaged view against
		// HEAD instead of the index, rendering a staged-new file as a whole
		// addition (it's absent from HEAD) rather than just its unstaged delta.
		return "", "", nil
	case 1:
		return args[0], "", nil
	case 2:
		return args[0], args[1], nil
	default:
		return "", "", fmt.Errorf("too many arguments: expected at most 2 commit refs")
	}
}

// DiffBetweenCommits lists the files changed between two refs. It shells out to
// git: a tree-to-tree diff is fast (it never walks the worktree) and git
// resolves refs correctly in every layout, including the linked worktrees where
// go-git can't (go-git#1842). It uses two args (not the `a..b` range, which
// rejects a tree on either side); for a root commit (base = the empty tree,
// whose object may not be in the odb) it uses `diff-tree --root` against target.
//
// --no-renames keeps a rename as a delete + an add (like statusFiles), so a
// renamed file in a commit drilldown renders correctly instead of as a whole
// addition — the new path doesn't exist at the base, so the diff engine would
// extract a /dev/null old side for it.
func (r *Repo) DiffBetweenCommits(baseRef, targetRef string) ([]ChangedFile, error) {
	var args []string
	if baseRef == EmptyTree {
		args = []string{"diff-tree", "--root", "--no-commit-id", "--name-status", "--no-renames", "-r", targetRef}
	} else {
		args = []string{"diff", "--name-status", "--no-renames", baseRef, targetRef}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return parseNameStatus(string(out)), nil
}

func matchPath(file string, paths []string) bool {
	for _, p := range paths {
		if file == p || strings.HasPrefix(file, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

func FilterByPaths(files []ChangedFile, paths []string) []ChangedFile {
	if len(paths) == 0 {
		return files
	}
	filtered := []ChangedFile{}
	for _, f := range files {
		if matchPath(f.Path, paths) {
			filtered = append(filtered, f)
		}
	}
	return filtered
}
