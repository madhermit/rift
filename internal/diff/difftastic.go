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
	return d.run(ctx, repoRoot, buildGitDiffArgs(opts, file), opts.Color)
}

func (d *difftasticEngine) DiffCommit(ctx context.Context, repoRoot, base, target string, color bool) (string, error) {
	return d.run(ctx, repoRoot, buildCommitDiffArgs(base, target, color), color)
}

func (d *difftasticEngine) run(ctx context.Context, repoRoot string, args []string, color bool) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	colorEnv := "DFT_COLOR=never"
	if color {
		colorEnv = "DFT_COLOR=always"
	}
	cmd.Env = append(cmd.Environ(), "GIT_EXTERNAL_DIFF="+d.path, colorEnv)
	return runGitDiff(cmd, "difftastic")
}
