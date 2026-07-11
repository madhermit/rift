package git

import (
	"fmt"
	"os"
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
// --no-renames matches statusFiles (a rename is a delete + an add on the real
// paths) so both paths agree, and -z keeps non-ASCII paths verbatim instead of
// C-quoted — otherwise the quoted key never matches a file's plain path.
func (r *Repo) addStats(files []ChangedFile, staged bool) {
	if len(files) == 0 {
		return // nothing to annotate; skip the git subprocess
	}
	args := []string{"diff", "--numstat", "--no-renames", "-z"}
	if staged {
		args = append(args, "--staged")
	}
	out, err := r.runGit(args...)
	if err != nil {
		return
	}
	applyNumstat(files, parseNumstatZ(out))
}

// applyNumstat copies added/deleted counts onto files matched by path.
func applyNumstat(files []ChangedFile, stats map[string][2]int) {
	for i := range files {
		if s, ok := stats[files[i].Path]; ok {
			files[i].Added, files[i].Deleted = s[0], s[1]
		}
	}
}

// parseNumstatZ parses `git diff --numstat -z --no-renames` output: NUL-
// terminated "added\tdeleted\tpath" records. Binary files report "-" for the
// counts, which Atoi maps to 0. -z keeps the path verbatim (no C-quoting).
func parseNumstatZ(out string) map[string][2]int {
	stats := make(map[string][2]int)
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		added, _ := strconv.Atoi(parts[0])   // "-" (binary) → 0
		deleted, _ := strconv.Atoi(parts[1]) // "-" (binary) → 0
		stats[parts[2]] = [2]int{added, deleted}
	}
	return stats
}

// parseNameStatusZ parses `git diff --name-status -z --no-renames` (and the
// equivalent diff-tree) output: NUL-separated fields alternating STATUS, PATH.
// With renames disabled there are no three-field rename records, so every entry
// is a clean pair and non-ASCII paths come through verbatim (no C-quoting).
func parseNameStatusZ(out string) []ChangedFile {
	fields := strings.Split(out, "\x00")
	var files []ChangedFile
	for i := 0; i+1 < len(fields); i += 2 {
		status, path := fields[i], fields[i+1]
		if status == "" || path == "" {
			continue
		}
		files = append(files, ChangedFile{
			Path:   path,
			Status: nameStatusCode(status),
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

// ListChanged lists the changed files for a diff scope, sorted by path and
// never nil. The scope picks the comparison:
//   - target set: the committed range base..target, tree to tree
//   - base set: the working tree against base, so committed changes since the
//     ref show up too (not just the worktree-vs-HEAD delta), matching a per-file
//     preview that also diffs against base
//   - neither: the working tree against the index (or the index against HEAD
//     when staged)
func (r *Repo) ListChanged(staged bool, base, target string) ([]ChangedFile, error) {
	var (
		files []ChangedFile
		err   error
	)
	switch {
	case target != "":
		files, err = r.DiffBetweenCommits(base, target)
	case base != "":
		files, err = r.DiffAgainstRef(base)
	default:
		files, err = r.ChangedFiles(staged)
	}
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []ChangedFile{}
	}
	SortByPath(files)
	return files, nil
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

// DiffBetweenCommits lists the files changed between two refs, with per-file
// line stats. It shells out to git: a tree-to-tree diff is fast (it never walks
// the worktree) and git resolves refs correctly in every layout, including the
// linked worktrees where go-git can't (go-git#1842). It uses two args (not the
// `a..b` range, which rejects a tree on either side).
//
// EmptyTree as the base means "diff target's whole content against nothing". For
// a parentless (root) commit that's `diff-tree --root`; git synthesizes the empty
// tree internally, so the object needn't be in the odb. For a non-root target,
// `diff-tree --root` would wrongly diff against the parent, so we diff against the
// real empty-tree object (resolved from git, since its hash is object-format
// dependent) instead.
//
// --no-renames keeps a rename as a delete + an add (like statusFiles), so a
// renamed file in a commit drilldown renders correctly instead of as a whole
// addition — the new path doesn't exist at the base, so the diff engine would
// extract a /dev/null old side for it.
func (r *Repo) DiffBetweenCommits(baseRef, targetRef string) ([]ChangedFile, error) {
	base, useRoot := baseRef, false
	if baseRef == EmptyTree {
		parentless, err := r.isParentless(targetRef)
		if err != nil {
			return nil, err
		}
		if parentless {
			useRoot = true
		} else if base, err = r.emptyTreeHash(); err != nil {
			return nil, err
		}
	}
	return changedFilesWithStats(func(format string) (string, error) {
		if useRoot {
			return r.runGit("diff-tree", "--root", "--no-commit-id", format, "--no-renames", "-z", "-r", targetRef)
		}
		return r.runGit("diff", format, "--no-renames", "-z", base, targetRef)
	})
}

// changedFilesWithStats runs the same NUL-separated diff twice — once for
// --name-status (path + change kind), once for --numstat (line counts) — and
// merges them into ChangedFiles. run selects the revisions; only the format flag
// differs between the two calls.
func changedFilesWithStats(run func(format string) (string, error)) ([]ChangedFile, error) {
	nameOut, err := run("--name-status")
	if err != nil {
		return nil, err
	}
	files := parseNameStatusZ(nameOut)

	numOut, err := run("--numstat")
	if err != nil {
		return nil, err
	}
	applyNumstat(files, parseNumstatZ(numOut))
	return files, nil
}

// isParentless reports whether ref has no parent commit (a root commit). git
// rev-list --parents -n 1 prints "<hash> <parent>..."; a lone hash means no
// parent.
func (r *Repo) isParentless(ref string) (bool, error) {
	out, err := r.runGit("rev-list", "--parents", "-n", "1", ref)
	if err != nil {
		return false, err
	}
	return len(strings.Fields(out)) <= 1, nil
}

// emptyTreeHash returns this repo's empty-tree object hash, writing the object if
// absent. The hash is object-format dependent (differs under SHA-256), so it must
// come from git rather than a hardcoded constant.
func (r *Repo) emptyTreeHash() (string, error) {
	out, err := r.runGit("hash-object", "-w", "-t", "tree", os.DevNull)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DiffAgainstRef lists the files that differ between the working tree and baseRef
// (like `git diff <ref>`), with per-file line stats. This is the single-ref diff
// scope: it surfaces both committed and uncommitted changes since baseRef, so the
// listing matches the per-file preview, which also diffs the working tree against
// baseRef. Untracked files are excluded, matching `git diff <ref>` semantics.
func (r *Repo) DiffAgainstRef(baseRef string) ([]ChangedFile, error) {
	return changedFilesWithStats(func(format string) (string, error) {
		return r.runGit("diff", format, "--no-renames", "-z", baseRef)
	})
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
