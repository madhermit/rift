package git

import (
	"fmt"
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v6"
)

type Repo struct {
	repo           *gogit.Repository
	root           string
	linkedWorktree bool
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

	root := wt.Filesystem.Root()
	return &Repo{
		repo:           r,
		root:           root,
		linkedWorktree: isLinkedWorktree(root),
	}, nil
}

// isLinkedWorktree detects bare-repo worktree layouts where .git is a file
// (containing a gitdir pointer) rather than a directory.
func isLinkedWorktree(root string) bool {
	info, err := os.Lstat(filepath.Join(root, ".git"))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func (r *Repo) Root() string {
	return r.root
}
