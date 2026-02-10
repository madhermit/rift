package git

import (
	"fmt"
	"os/exec"
	"strings"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

type ChangedFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func (r *Repo) ChangedFiles(staged bool) ([]ChangedFile, error) {
	if r.linkedWorktree {
		return r.changedFilesShell(staged)
	}
	files, err := r.changedFilesGoGit(staged)
	if err != nil {
		return r.changedFilesShell(staged)
	}
	return files, nil
}

func (r *Repo) changedFilesGoGit(staged bool) ([]ChangedFile, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return nil, err
	}

	status, err := wt.Status()
	if err != nil {
		return nil, err
	}

	var files []ChangedFile
	for path, s := range status {
		var code string
		if staged {
			if s.Staging == '?' || s.Staging == ' ' || s.Staging == 0 {
				continue
			}
			code = statusCodeToString(s.Staging)
		} else {
			code = statusCodeToString(s.Worktree)
		}
		if code == "" {
			continue
		}
		files = append(files, ChangedFile{Path: path, Status: code})
	}

	return files, nil
}

// changedFilesShell falls back to git diff when go-git can't compute
// status correctly (e.g. in linked worktree layouts).
func (r *Repo) changedFilesShell(staged bool) ([]ChangedFile, error) {
	args := []string{"diff"}
	if staged {
		args = append(args, "--staged")
	}
	args = append(args, "--name-status")
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	return parseNameStatus(string(out)), nil
}

func parseNameStatus(out string) []ChangedFile {
	var files []ChangedFile
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		files = append(files, ChangedFile{
			Path:   parts[1],
			Status: nameStatusCode(parts[0]),
		})
	}
	return files
}

func nameStatusCode(code string) string {
	switch {
	case strings.HasPrefix(code, "M"):
		return "Modified"
	case strings.HasPrefix(code, "A"):
		return "Added"
	case strings.HasPrefix(code, "D"):
		return "Deleted"
	case strings.HasPrefix(code, "R"):
		return "Renamed"
	case strings.HasPrefix(code, "C"):
		return "Copied"
	default:
		return code
	}
}

func DiffTargets(args []string) (base, target string, err error) {
	switch len(args) {
	case 0:
		return "HEAD", "", nil
	case 1:
		return args[0], "", nil
	case 2:
		return args[0], args[1], nil
	default:
		return "", "", fmt.Errorf("too many arguments: expected at most 2 commit refs")
	}
}

func (r *Repo) DiffBetweenCommits(baseRef, targetRef string) ([]ChangedFile, error) {
	baseCommit, err := r.resolveCommit(baseRef)
	if err != nil {
		return nil, fmt.Errorf("resolve base %q: %w", baseRef, err)
	}

	targetCommit, err := r.resolveCommit(targetRef)
	if err != nil {
		return nil, fmt.Errorf("resolve target %q: %w", targetRef, err)
	}

	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get base tree: %w", err)
	}

	targetTree, err := targetCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get target tree: %w", err)
	}

	changes, err := baseTree.Diff(targetTree)
	if err != nil {
		return nil, fmt.Errorf("diff trees: %w", err)
	}

	var files []ChangedFile
	for _, c := range changes {
		name := c.To.Name
		if name == "" {
			name = c.From.Name
		}
		files = append(files, ChangedFile{
			Path:   name,
			Status: diffActionString(c),
		})
	}

	return files, nil
}

func (r *Repo) resolveCommit(ref string) (*object.Commit, error) {
	h, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, err
	}
	return r.repo.CommitObject(*h)
}

func diffActionString(c *object.Change) string {
	from := c.From.Name
	to := c.To.Name
	switch {
	case from == "" && to != "":
		return "Added"
	case from != "" && to == "":
		return "Deleted"
	default:
		return "Modified"
	}
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

func statusCodeToString(c gogit.StatusCode) string {
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
	case ' ':
		return ""
	default:
		if c == 0 {
			return ""
		}
		return string(c)
	}
}
