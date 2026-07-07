package git

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

type CommitInfo struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
	Body    string `json:"body,omitempty"`
}

func (r *Repo) Log(ref string, maxCount int, paths []string) ([]CommitInfo, error) {
	// Commit ranges (a..b, a...b) can't be resolved to a single revision by
	// go-git, so route them straight to git log rather than relying on
	// ResolveRevision to reject them.
	if strings.Contains(ref, "..") {
		return logShell(r.root, ref, maxCount, false, paths)
	}

	h, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return logShell(r.root, ref, maxCount, false, paths)
	}

	commits, err := r.logGoGit(*h, maxCount, paths)
	if err != nil {
		return logShell(r.root, ref, maxCount, false, paths)
	}
	return commits, nil
}

// LogAll lists commits across all refs. It always shells out to `git log --all`:
// go-git returns zero refs in bare-repo linked-worktree layouts (go-git#1842)
// without erroring, so a go-git path would silently show nothing there; git also
// handles the commit-time ordering, --max-count, tag-only and detached-HEAD
// commits that a per-ref walk misses.
func (r *Repo) LogAll(maxCount int, paths []string) ([]CommitInfo, error) {
	return logShell(r.root, "", maxCount, true, paths)
}

func (r *Repo) logGoGit(from plumbing.Hash, maxCount int, paths []string) ([]CommitInfo, error) {
	opts := &gogit.LogOptions{
		From:  from,
		Order: gogit.LogOrderCommitterTime,
	}
	if len(paths) > 0 {
		opts.PathFilter = func(file string) bool { return matchPath(file, paths) }
	}

	iter, err := r.repo.Log(opts)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	commits := []CommitInfo{}
	err = iter.ForEach(func(c *object.Commit) error {
		if maxCount > 0 && len(commits) >= maxCount {
			return storer.ErrStop
		}
		commits = append(commits, commitToInfo(c))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return commits, nil
}

// logShell falls back to shelling out to git log when go-git can't handle the
// request: bare-repo worktree layouts, or commit ranges (a..b) that go-git
// can't resolve. dir is the repo root the command runs in.
func logShell(dir, ref string, maxCount int, all bool, paths []string) ([]CommitInfo, error) {
	const fieldSep = "\x1e"
	const recordSep = "\x00"
	// Use git's %xNN escapes so no special bytes appear in the argument itself.
	args := []string{"log", "--format=%h%x1e%an%x1e%ai%x1e%s%x1e%b%x00"}
	if maxCount > 0 {
		args = append(args, "-n", strconv.Itoa(maxCount))
	}
	if all {
		args = append(args, "--all")
	} else if ref != "" {
		args = append(args, ref)
	}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if isUnbornHead(ref, err) {
			return nil, errors.New("no commits yet")
		}
		return nil, gitErr("git log", err)
	}
	return parseGitLogOutput(string(out), fieldSep, recordSep), nil
}

// isUnbornHead reports whether a `git log` failure is the empty-repo case: HEAD
// points at a branch with no commits yet. The default log shells `git log HEAD`,
// which fails as an ambiguous 'HEAD' rather than the bare "no commits yet", so we
// accept both — but only for the default HEAD scope, so an explicit bad ref keeps
// surfacing git's real "unknown revision" error.
func isUnbornHead(ref string, err error) bool {
	if ref != "" && ref != "HEAD" {
		return false
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return false
	}
	msg := string(ee.Stderr)
	return strings.Contains(msg, "does not have any commits yet") ||
		strings.Contains(msg, "ambiguous argument 'HEAD'")
}

func parseGitLogOutput(out, fieldSep, recordSep string) []CommitInfo {
	var commits []CommitInfo
	for _, record := range strings.Split(out, recordSep) {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, fieldSep, 5)
		if len(parts) < 4 {
			continue
		}
		ci := CommitInfo{
			Hash:    parts[0],
			Author:  parts[1],
			Date:    formatShellDate(parts[2]),
			Message: parts[3],
		}
		if len(parts) == 5 {
			ci.Body = strings.TrimSpace(parts[4])
		}
		commits = append(commits, ci)
	}
	return commits
}

// formatShellDate trims "%ai" output ("2025-01-15 10:30:00 -0500") to "2025-01-15 10:30".
func formatShellDate(s string) string {
	if len(s) >= 16 {
		return s[:16]
	}
	return s
}

func commitToInfo(c *object.Commit) CommitInfo {
	msg := strings.TrimRight(c.Message, "\n")
	subject, body, _ := strings.Cut(msg, "\n")
	return CommitInfo{
		Hash:    c.Hash.String()[:7],
		Author:  c.Author.Name,
		Date:    c.Author.When.Format("2006-01-02 15:04"),
		Message: subject,
		Body:    strings.TrimSpace(body),
	}
}
