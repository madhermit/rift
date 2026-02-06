package diff

import (
	"context"
	"os/exec"
)

type fallbackEngine struct{}

func (f *fallbackEngine) Name() string { return "git-diff" }

func (f *fallbackEngine) Diff(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error) {
	args := buildGitDiffArgs(opts, file)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	return runGitDiff(cmd, "git diff")
}
