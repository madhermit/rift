package git

import (
	"fmt"
	"os/exec"
	"strings"

	gogit "github.com/go-git/go-git/v6"
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

	return &Repo{repo: r, root: wt.Filesystem.Root()}, nil
}

func (r *Repo) Root() string {
	return r.root
}

// CurrentBranch returns the checked-out branch name, the short commit hash when
// HEAD is detached, or "" if it can't be determined. go-git can't read HEAD in a
// linked worktree (go-git#1842), so any go-git failure falls back to shelling —
// the same try-go-git-then-shell pattern the log reads use.
func (r *Repo) CurrentBranch() string {
	if r == nil {
		return ""
	}
	if head, err := r.repo.Head(); err == nil {
		if head.Name().IsBranch() {
			return head.Name().Short()
		}
		return head.Hash().String()[:7] // detached HEAD
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
