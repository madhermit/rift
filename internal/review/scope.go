package review

import (
	"os"
	"path/filepath"

	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
)

// fileScope is a changed file paired with its extractor, the new-side content to
// parse, the old-side content (nil when there's none, e.g. a root commit or an
// untracked file) used to tell a rename from a decorator-only decl change, and
// the set of line numbers the diff added or modified.
type fileScope struct {
	Path    string
	Ext     Extractor
	Content []byte
	Old     []byte
	Added   map[int]bool
}

// gather collects the changed files in scope, keeping only those with a
// supported extractor and a readable new-side version. It serves either a
// working-tree/staged diff or a committed range, depending on the scope.
func gather(repo *git.Repo, scope DiffScope) ([]fileScope, error) {
	if scope.Committed() {
		return gatherRange(repo, scope.Base, scope.Target, scope.Paths)
	}
	return gatherWorktree(repo, scope.Staged, scope.Paths)
}

func gatherWorktree(repo *git.Repo, staged bool, paths []string) ([]fileScope, error) {
	files, err := repo.ChangedFiles(staged)
	if err != nil {
		return nil, err
	}
	root := repo.Root()
	// One un-pathspec'd diff, parsed once and indexed by path: this keeps git's
	// rename detection on (a per-file pathspec defeats it, so a renamed test file
	// reads as 100% added and floods the lens) and spends a single subprocess for
	// the whole changeset instead of one per file.
	addedByPath := addedLinesByPath(orEmpty(diff.RawWorktreeDiff(root, staged)))
	return collect(git.FilterByPaths(files, paths), func(f git.ChangedFile) ([]byte, []byte, map[int]bool, bool) {
		content, err := worktreeContent(root, staged, f.Path)
		if err != nil {
			return nil, nil, nil, false
		}
		added := addedByPath[f.Path]
		if added == nil {
			// Untracked files aren't in `git diff`; the whole file is new.
			if f.Status == "Untracked" {
				added = allLines(content)
			} else {
				added = map[int]bool{}
			}
		}
		return content, worktreeOld(root, staged, f), added, true
	}), nil
}

func gatherRange(repo *git.Repo, base, target string, paths []string) ([]fileScope, error) {
	files, err := repo.DiffBetweenCommits(base, target)
	if err != nil {
		return nil, err
	}
	root := repo.Root()
	// A root commit (empty-tree base) adds the whole file; otherwise index a single
	// rename-detecting range diff by path (see gatherWorktree).
	var addedByPath map[string]map[int]bool
	if base != git.EmptyTree {
		addedByPath = addedLinesByPath(orEmpty(diff.RawRangeDiffAll(root, base, target)))
	}
	return collect(git.FilterByPaths(files, paths), func(f git.ChangedFile) ([]byte, []byte, map[int]bool, bool) {
		content, err := diff.ShowFile(root, target, f.Path)
		if err != nil {
			return nil, nil, nil, false
		}
		if base == git.EmptyTree {
			return content, nil, allLines(content), true
		}
		added := addedByPath[f.Path]
		if added == nil {
			added = map[int]bool{}
		}
		// A renamed file's old side lives at its old path in the base — without
		// it every spec in the file would read as added rather than renamed.
		oldPath := f.Path
		if f.OldPath != "" {
			oldPath = f.OldPath
		}
		old, _ := diff.ShowFile(root, base, oldPath) // nil old side for a file added in the range
		return content, old, added, true
	}), nil
}

// collect builds a fileScope for each changed file with a supported extractor,
// resolving its new-side content, old-side content, and added-line set. Files
// without an extractor, deleted files, and ones resolve reports unusable
// (ok=false) are skipped, per graceful degradation.
func collect(files []git.ChangedFile, resolve func(git.ChangedFile) (content, old []byte, added map[int]bool, ok bool)) []fileScope {
	var scopes []fileScope
	for _, f := range files {
		ext := extractorFor(f.Path)
		if f.Status == "Deleted" || ext == nil {
			continue
		}
		content, old, added, ok := resolve(f)
		if !ok {
			continue
		}
		scopes = append(scopes, fileScope{Path: f.Path, Ext: ext, Content: content, Old: old, Added: added})
	}
	return scopes
}

// orEmpty maps a diff-fetch error to the empty diff so a single failed subprocess
// degrades to an empty added-line index rather than aborting the whole lens.
func orEmpty(raw string, err error) string {
	if err != nil {
		return ""
	}
	return raw
}

// worktreeContent returns the new side of a working-tree change: the file on
// disk for an unstaged diff, or the index version (git show :path) when staged.
func worktreeContent(root string, staged bool, path string) ([]byte, error) {
	if staged {
		return diff.ShowFile(root, "", path)
	}
	return os.ReadFile(filepath.Join(root, path))
}

// worktreeOld returns the old side of a working-tree change for name-change
// detection: HEAD when staged, the index version otherwise. An untracked file
// has no old side. A missing old side (nil) just means every touched spec reads
// as new.
func worktreeOld(root string, staged bool, f git.ChangedFile) []byte {
	if f.Status == "Untracked" {
		return nil
	}
	ref := "" // index version
	if staged {
		ref = "HEAD"
	}
	old, _ := diff.ShowFile(root, ref, f.Path)
	return old
}

// allLines marks every line of the file (a root commit adds the whole file),
// blank lines included — classify() walks a test's whole line range, so a blank
// line inside a new test must count as added or the test reads as a partial
// (renamed/modified) change rather than wholly new. A trailing newline doesn't
// invent a phantom line past the end.
func allLines(content []byte) map[int]bool {
	lines := make(map[int]bool)
	n := 1
	for i, b := range content {
		lines[n] = true
		if b == '\n' && i < len(content)-1 {
			n++
		}
	}
	return lines
}

// addedLinesByPath parses a whole-changeset diff once and returns each file's
// added/modified line set, keyed by the file's (new-side) path.
func addedLinesByPath(raw string) map[string]map[int]bool {
	byPath := make(map[string]map[int]bool)
	for _, fd := range diff.ParseUnifiedDiff(raw) {
		byPath[fd.Path] = hunkAddedLines(fd.Hunks)
	}
	return byPath
}

// hunkAddedLines maps a file's hunks to the new-side lines it touched. Additions
// mark the line they land on. A deletion consumes no new-side line, but a
// deletion-only change (e.g. removing an assertion) would otherwise leave the
// weakened test with an empty added set and never surface — so a deletion also
// marks the new-side line it abuts, mapping it to the enclosing spec.
func hunkAddedLines(hunks []diff.Hunk) map[int]bool {
	added := make(map[int]bool)
	for _, h := range hunks {
		line := h.NewStart
		for _, l := range h.Lines {
			if l == "" {
				continue
			}
			switch l[0] {
			case '+':
				added[line] = true
				line++
			case ' ':
				line++
			case '-':
				added[line] = true // surface the deletion at its new-side position
			case '\\':
				// "no newline at end of file" marker: no new-side line consumed
			}
		}
	}
	return added
}
