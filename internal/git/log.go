package git

import (
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
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
		return nil, fmt.Errorf("resolve ref %q: %w", ref, err)
	}

	opts := &gogit.LogOptions{
		From:  *h,
		Order: gogit.LogOrderCommitterTime,
	}
	if len(paths) > 0 {
		opts.PathFilter = func(file string) bool { return matchPath(file, paths) }
	}

	iter, err := r.repo.Log(opts)
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
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
		return nil, fmt.Errorf("iterate commits: %w", err)
	}

	return commits, nil
}

func (r *Repo) LogAll(maxCount int, paths []string) ([]CommitInfo, error) {
	refs, err := r.repo.References()
	if err != nil {
		return nil, fmt.Errorf("list references: %w", err)
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
		return nil, fmt.Errorf("iterate all commits: %w", err)
	}

	return commits, nil
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
