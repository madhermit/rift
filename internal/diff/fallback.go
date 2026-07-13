package diff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type fallbackEngine struct{}

func (f *fallbackEngine) Name() string { return "git-diff" }

// Diff shells `git diff` for one file. An untracked file yields no output there
// (git diff only covers tracked content), so an empty worktree-scope result is
// re-checked: if the file is untracked, it's diffed against the null device
// instead so its content renders as a new-file diff — matching the difftastic
// engine, which reaches the same result via its failed `git show :file`.
func (f *fallbackEngine) Diff(ctx context.Context, repoRoot string, file File, opts DiffOpts) (string, error) {
	display := displayFlags(opts.Color)
	args := buildGitDiffArgs(opts, file, display)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	out, err := runGitDiff(cmd, "git diff")
	if err != nil || strings.TrimSpace(out) != "" {
		return out, err
	}
	worktreeScope := !opts.Staged && opts.Base == "" && opts.Target == ""
	if !worktreeScope || !isUntracked(ctx, repoRoot, file.Path) {
		return out, nil
	}
	args = append(buildGitDiffArgs(opts, File{}, display), "--no-index", "--", os.DevNull, file.Path)
	cmd = exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	return runGitDiff(cmd, "git diff untracked")
}

// isUntracked reports whether file is absent from the index (`ls-files
// --error-unmatch` exits non-zero for it). Only consulted when a worktree diff
// came back empty, to tell "unchanged" from "untracked".
func isUntracked(ctx context.Context, repoRoot, file string) bool {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--error-unmatch", "--", file)
	cmd.Dir = repoRoot
	return cmd.Run() != nil
}

func (f *fallbackEngine) DiffHunks(_ context.Context, hunks []Hunk, _, _ string, color bool, _ int) []string {
	results := make([]string, len(hunks))
	for i, h := range hunks {
		if !color {
			results[i] = h.rawRender()
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
