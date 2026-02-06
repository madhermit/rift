package git

import (
	"fmt"

	gogit "github.com/go-git/go-git/v5"
)

type Repo struct {
	repo *gogit.Repository
	root string
}

func OpenRepo() (*Repo, error) {
	r, err := gogit.PlainOpenWithOptions(".", &gogit.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("get worktree: %w", err)
	}

	return &Repo{
		repo: r,
		root: wt.Filesystem.Root(),
	}, nil
}

func (r *Repo) Root() string {
	return r.root
}
