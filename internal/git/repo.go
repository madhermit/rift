package git

import (
	"errors"
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
		if errors.Is(err, gogit.ErrRepositoryNotExists) {
			// go-git can't find a worktree here. Running inside a bare git dir
			// (e.g. a `.bare` layout) surfaces the same error, so ask git to
			// disambiguate rather than repeat go-git's misleading message.
			if runningInBareRepo() {
				return nil, errors.New("bare repository — run from a worktree")
			}
			return nil, errors.New("not a git repository (or any parent directory)")
		}
		return nil, fmt.Errorf("open repo: %w", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		if errors.Is(err, gogit.ErrIsBareRepository) {
			return nil, errors.New("bare repository — run from a worktree")
		}
		return nil, fmt.Errorf("get worktree: %w", err)
	}

	return &Repo{repo: r, root: wt.Filesystem.Root()}, nil
}

// runningInBareRepo reports whether the current directory sits inside a bare git
// directory, so OpenRepo can give a "run from a worktree" hint instead of a flat
// "not a git repository".
func runningInBareRepo() bool {
	out, err := exec.Command("git", "rev-parse", "--is-bare-repository").Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// runGit runs `git <args>` in the repo root and returns stdout, wrapping any
// failure with the command's stderr (git's actual fatal message) via gitErr.
// The reads that use it (status, name-status, numstat, rev-parse, stash list)
// exit 0 whether or not there are changes, so a non-zero status is a real error.
func (r *Repo) runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	out, err := cmd.Output()
	if err != nil {
		return "", gitErr("git "+strings.Join(args, " "), err)
	}
	return string(out), nil
}

// gitErr wraps a shelled-git failure with the command's stderr. cmd.Output()
// captures stderr into ExitError.Stderr, so an opaque "exit status 128" carries
// git's actual fatal message (e.g. a bad revision or wrong work tree).
func gitErr(label string, err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			return fmt.Errorf("%s: %w: %s", label, err, msg)
		}
	}
	return fmt.Errorf("%s: %w", label, err)
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
