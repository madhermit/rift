package review

import (
	"os"
	"path/filepath"

	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
)

// fileScope is a changed file paired with its extractor, the new-side content
// to parse, and the set of line numbers the diff added or modified.
type fileScope struct {
	Path    string
	Ext     Extractor
	Content []byte
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
	return collect(git.FilterByPaths(files, paths), func(f git.ChangedFile) ([]byte, map[int]bool, bool) {
		content, err := worktreeContent(root, staged, f.Path)
		if err != nil {
			return nil, nil, false
		}
		return content, worktreeAdded(root, staged, f), true
	}), nil
}

func gatherRange(repo *git.Repo, base, target string, paths []string) ([]fileScope, error) {
	files, err := repo.DiffBetweenCommits(base, target)
	if err != nil {
		return nil, err
	}
	root := repo.Root()
	return collect(git.FilterByPaths(files, paths), func(f git.ChangedFile) ([]byte, map[int]bool, bool) {
		content, err := diff.ShowFile(root, target, f.Path)
		if err != nil {
			return nil, nil, false
		}
		return content, rangeAdded(root, base, target, f.Path, content), true
	}), nil
}

// collect builds a fileScope for each changed file with a supported extractor,
// resolving its new-side content and added-line set. Files without an extractor,
// deleted files, and ones resolve reports unusable (ok=false) are skipped, per
// graceful degradation.
func collect(files []git.ChangedFile, resolve func(git.ChangedFile) (content []byte, added map[int]bool, ok bool)) []fileScope {
	var scopes []fileScope
	for _, f := range files {
		ext := extractorFor(f.Path)
		if f.Status == "Deleted" || ext == nil {
			continue
		}
		content, added, ok := resolve(f)
		if !ok {
			continue
		}
		scopes = append(scopes, fileScope{Path: f.Path, Ext: ext, Content: content, Added: added})
	}
	return scopes
}

func worktreeAdded(root string, staged bool, f git.ChangedFile) map[int]bool {
	raw, err := worktreeRawDiff(root, staged, f)
	if err != nil {
		return map[int]bool{}
	}
	return addedLines(raw)
}

func worktreeRawDiff(root string, staged bool, f git.ChangedFile) (string, error) {
	if f.Status == "Untracked" {
		return diff.RawNewFileDiff(root, f.Path)
	}
	return diff.RawUnifiedDiff(root, staged, f.Path)
}

// worktreeContent returns the new side of a working-tree change: the file on
// disk for an unstaged diff, or the index version (git show :path) when staged.
func worktreeContent(root string, staged bool, path string) ([]byte, error) {
	if staged {
		return diff.ShowFile(root, "", path)
	}
	return os.ReadFile(filepath.Join(root, path))
}

// rangeAdded returns the new-side lines a committed range introduced. A root
// commit (empty-tree base) adds the whole file, so every line counts.
func rangeAdded(root, base, target, path string, content []byte) map[int]bool {
	if base == git.EmptyTree {
		return allLines(content)
	}
	raw, err := diff.RawRangeDiff(root, base, target, path)
	if err != nil {
		return map[int]bool{}
	}
	return addedLines(raw)
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

// addedLines returns the set of new-side line numbers added or modified by the
// diff. A test spec counts as touched when any of its lines is in this set.
func addedLines(raw string) map[int]bool {
	added := make(map[int]bool)
	for _, fd := range diff.ParseUnifiedDiff(raw) {
		for _, h := range fd.Hunks {
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
				case '-', '\\':
					// deletion / "no newline" marker: no new-side line consumed
				}
			}
		}
	}
	return added
}
