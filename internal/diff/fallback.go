package diff

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type fallbackEngine struct{}

func (f *fallbackEngine) Name() string { return "git-diff" }

func (f *fallbackEngine) Diff(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error) {
	args := buildGitDiffArgs(opts, file)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	return runGitDiff(cmd, "git diff")
}

func (f *fallbackEngine) DiffCommit(ctx context.Context, repoRoot, base, target string, color bool, width int) (string, error) {
	args := buildCommitDiffArgs(base, target, color)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	return runGitDiff(cmd, "git diff commit")
}

func (f *fallbackEngine) DiffHunks(_ context.Context, hunks []Hunk, _, _ string, color bool, _ int) []string {
	results := make([]string, len(hunks))
	for i, h := range hunks {
		if !color {
			results[i] = h.Header + "\n" + strings.Join(h.Lines, "\n")
			continue
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("\x1b[36m%s\x1b[0m\n", h.Header))
		for _, line := range h.Lines {
			if len(line) == 0 {
				b.WriteString("\n")
				continue
			}
			switch line[0] {
			case '+':
				fmt.Fprintf(&b, "\x1b[32m%s\x1b[0m\n", line)
			case '-':
				fmt.Fprintf(&b, "\x1b[31m%s\x1b[0m\n", line)
			default:
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		results[i] = strings.TrimSuffix(b.String(), "\n")
	}
	return results
}
