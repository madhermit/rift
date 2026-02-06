package diff

import (
	"context"
	"fmt"
	"os/exec"
)

type DiffOpts struct {
	Staged bool
	Base   string
	Target string
	Color  bool
}

type Engine interface {
	Diff(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error)
	Name() string
}

func NewEngine() Engine {
	path, err := FindDifft()
	if err == nil && path != "" {
		return &difftasticEngine{path: path}
	}
	return &fallbackEngine{}
}

func buildGitDiffArgs(opts DiffOpts, file string) []string {
	args := []string{"diff"}
	if opts.Color {
		args = append(args, "--color=always")
	} else {
		args = append(args, "--color=never")
	}
	if opts.Staged {
		args = append(args, "--staged")
	} else if opts.Base != "" && opts.Target != "" {
		args = append(args, opts.Base, opts.Target)
	} else if opts.Base != "" {
		args = append(args, opts.Base)
	}
	args = append(args, "--", file)
	return args
}

func runGitDiff(cmd *exec.Cmd, label string) (string, error) {
	out, err := cmd.Output()
	if err != nil {
		// git diff exits 1 when there are differences — that's not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return string(out), nil
		}
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return string(out), nil
}
