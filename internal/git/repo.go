package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// CurrentBranch returns the checked-out branch name, the short commit hash when
// HEAD is detached, or "" if it can't be determined. go-git can't resolve HEAD
// in linked worktrees (go-git#1842), so those (and any go-git failure) shell out.
func (r *Repo) CurrentBranch() string {
	if r == nil {
		return ""
	}
	if !r.linkedWorktree {
		if head, err := r.repo.Head(); err == nil {
			if head.Name().IsBranch() {
				return head.Name().Short()
			}
			return head.Hash().String()[:7] // detached HEAD
		}
	}
	out, err := exec.Command("git", "-C", r.root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	if name := strings.TrimSpace(string(out)); name != "HEAD" {
		return name
	}
	if h, err := exec.Command("git", "-C", r.root, "rev-parse", "--short", "HEAD").Output(); err == nil {
		return strings.TrimSpace(string(h))
	}
	return ""
}
