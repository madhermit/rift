package git

import (
	"fmt"
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
	h, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return logShell(ref, maxCount, false, paths)
	}

	commits, err := r.logGoGit(*h, maxCount, paths)
	if err != nil {
		return logShell(ref, maxCount, false, paths)
	}
	return commits, nil
}

func (r *Repo) LogAll(maxCount int, paths []string) ([]CommitInfo, error) {
	commits, err := r.logAllGoGit(maxCount, paths)
	if err != nil {
		return logShell("", maxCount, true, paths)
	}
	return commits, nil
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

func (r *Repo) logAllGoGit(maxCount int, paths []string) ([]CommitInfo, error) {
	refs, err := r.repo.References()
	if err != nil {
		return nil, err
	}

	seen := map[plumbing.Hash]bool{}
	commits := []CommitInfo{}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if !ref.Name().IsBranch() && !ref.Name().IsRemote() {
			return nil
		}
		opts := &gogit.LogOptions{
			From:  ref.Hash(),
			Order: gogit.LogOrderCommitterTime,
		}
		if len(paths) > 0 {
			opts.PathFilter = func(file string) bool { return matchPath(file, paths) }
		}
		iter, err := r.repo.Log(opts)
		if err != nil {
			return nil
		}
		defer iter.Close()

		return iter.ForEach(func(c *object.Commit) error {
			if maxCount > 0 && len(commits) >= maxCount {
				return storer.ErrStop
			}
			if seen[c.Hash] {
				return nil
			}
			seen[c.Hash] = true
			commits = append(commits, commitToInfo(c))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return commits, nil
}

// logShell falls back to shelling out to git log when go-git can't handle
// the repo layout (e.g. bare-repo worktree setups).
func logShell(ref string, maxCount int, all bool, paths []string) ([]CommitInfo, error) {
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
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	return parseGitLogOutput(string(out), fieldSep, recordSep), nil
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

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
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
