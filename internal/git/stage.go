package git

import (
	"bytes"
	"fmt"
	"os/exec"
)

func (r *Repo) Stage(paths ...string) error {
	args := append([]string{"-C", r.root, "add", "--"}, paths...)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", out, err)
	}
	return nil
}

func (r *Repo) Unstage(paths ...string) error {
	args := append([]string{"-C", r.root, "restore", "--staged", "--"}, paths...)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git restore --staged: %s: %w", out, err)
	}
	return nil
}

func (r *Repo) StageHunk(patch string) error {
	cmd := exec.Command("git", "-C", r.root, "apply", "--cached", "--unidiff-zero", "-")
	cmd.Stdin = bytes.NewReader([]byte(patch))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git apply --cached: %s: %w", out, err)
	}
	return nil
}

func (r *Repo) UnstageHunk(patch string) error {
	cmd := exec.Command("git", "-C", r.root, "apply", "--cached", "--reverse", "--unidiff-zero", "-")
	cmd.Stdin = bytes.NewReader([]byte(patch))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git apply --cached --reverse: %s: %w", out, err)
	}
	return nil
}
