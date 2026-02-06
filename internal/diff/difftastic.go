package diff

import (
	"context"
	"fmt"
	"os/exec"
)

func FindDifft() (string, error) {
	path, err := exec.LookPath("difft")
	if err != nil {
		return "", fmt.Errorf("difft not found: %w", err)
	}
	return path, nil
}

type difftasticEngine struct {
	path string
}

func (d *difftasticEngine) Name() string { return "difftastic" }

func (d *difftasticEngine) Diff(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error) {
	args := buildGitDiffArgs(opts, file)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	cmd.Env = append(cmd.Environ(), "GIT_EXTERNAL_DIFF="+d.path)
	return runGitDiff(cmd, "difftastic diff")
}
