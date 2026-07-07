package diff

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type difftasticEngine struct {
	path string
}

func (d *difftasticEngine) Name() string { return "difftastic" }

func (d *difftasticEngine) Diff(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error) {
	if opts.Width <= 0 {
		return d.diffViaGit(ctx, repoRoot, file, opts)
	}
	return d.diffDirect(ctx, repoRoot, file, opts)
}

func (d *difftasticEngine) diffViaGit(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error) {
	args := buildGitDiffArgs(opts, file, false)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	colorEnv := "DFT_COLOR=never"
	if opts.Color {
		colorEnv = "DFT_COLOR=always"
	}
	cmd.Env = append(cmd.Environ(), "GIT_EXTERNAL_DIFF="+d.path, colorEnv)
	return runGitDiff(cmd, "difftastic")
}

func (d *difftasticEngine) diffDirect(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error) {
	tmpDir, err := os.MkdirTemp("", "rift-diff-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Old side is always extracted from a git ref; new side is either
	// extracted (base+target, staged) or the working tree file.
	var oldRef string
	var newPath string
	switch {
	case opts.Base != "" && opts.Target != "":
		oldRef = opts.Base
		newPath = showOrNull(ctx, repoRoot, opts.Target, file, filepath.Join(tmpDir, "b", file))
	case opts.Staged:
		oldRef = "HEAD"
		newPath = showOrNull(ctx, repoRoot, "", file, filepath.Join(tmpDir, "b", file))
	case opts.Base != "":
		oldRef = opts.Base
		newPath = worktreeOrNull(filepath.Join(repoRoot, file))
	default:
		newPath = worktreeOrNull(filepath.Join(repoRoot, file))
	}

	oldPath := showOrNull(ctx, repoRoot, oldRef, file, filepath.Join(tmpDir, "a", file))
	return d.diffFiles(ctx, oldPath, newPath, opts.Color, opts.Width, opts.Display)
}

// diffFiles calls difft directly in 2-arg mode. Note: difft ignores --width
// for pure additions (old=/dev/null) even in side-by-side mode. Callers should
// hard-wrap the output as a safety net. See https://github.com/Wilfred/difftastic/issues/861
func (d *difftasticEngine) diffFiles(ctx context.Context, oldPath, newPath string, color bool, width int, display Display) (string, error) {
	args := []string{"--display", display.difftValue(width), "--tab-width", "4"}
	if width > 0 {
		args = append(args, "--width", strconv.Itoa(width))
	}
	if color {
		args = append(args, "--color", "always")
	} else {
		args = append(args, "--color", "never")
	}
	args = append(args, oldPath, newPath)

	cmd := exec.CommandContext(ctx, d.path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		// difft exits 1 when there are differences — that's not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return stdout.String(), nil
		}
		return "", fmt.Errorf("difft %s %s: %w: %s", oldPath, newPath, err, stderr.String())
	}
	return stdout.String(), nil
}

// DiffHunks renders each hunk individually through difftastic by applying each
// hunk to the full base file. This gives tree-sitter the full file context for
// accurate syntax-aware diffs. The base file is written once and the hunks are
// rendered concurrently (bounded to the CPU count via ParallelStream), keeping
// output in hunk order. Falls back to raw lines if difft fails.
func (d *difftasticEngine) DiffHunks(ctx context.Context, hunks []Hunk, filename, baseContent string, color bool, width int) []string {
	results := make([]string, len(hunks))

	tmpDir, err := os.MkdirTemp("", "rift-hunks-*")
	if err != nil {
		return rawHunks(hunks)
	}
	defer os.RemoveAll(tmpDir)

	// Name the temp files with the real basename so difftastic detects the
	// language by filename — extensionless files like Makefile or Containerfile
	// would otherwise render as plain text. The base file is shared across hunks;
	// each hunk's new side lives in its own subdir to avoid a collision.
	base := filepath.Base(filename)
	if base == "." || base == string(filepath.Separator) {
		base = "f"
	}
	basePath := filepath.Join(tmpDir, "base", base)
	if err := writeTemp(basePath, []byte(baseContent)); err != nil {
		return rawHunks(hunks)
	}

	i := 0
	for rendered := range ParallelStream(len(hunks), func(i int) string {
		h := hunks[i]
		newPath := filepath.Join(tmpDir, strconv.Itoa(i), base)
		out, err := d.diffHunk(ctx, basePath, newPath, ApplyHunk(baseContent, h), color, width)
		if err != nil || strings.TrimSpace(out) == "" {
			return h.rawRender()
		}
		return out
	}) {
		results[i] = rendered
		i++
	}
	return results
}

// diffHunk writes a hunk's new-side content and renders it against the shared
// base file. Hunks are small; inline keeps them readable regardless of pane width.
func (d *difftasticEngine) diffHunk(ctx context.Context, basePath, newPath, newContent string, color bool, width int) (string, error) {
	if err := writeTemp(newPath, []byte(newContent)); err != nil {
		return "", err
	}
	return d.diffFiles(ctx, basePath, newPath, color, width, DisplayInline)
}

// rawHunks is the graceful-degradation output when the scratch dir can't be set
// up: each hunk's plain header + body.
func rawHunks(hunks []Hunk) []string {
	out := make([]string, len(hunks))
	for i, h := range hunks {
		out[i] = h.rawRender()
	}
	return out
}

// writeTemp writes content to destPath, creating its parent directory. Used for
// the base and per-hunk scratch files difftastic diffs.
func writeTemp(destPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0700); err != nil {
		return err
	}
	return os.WriteFile(destPath, content, 0600)
}

func showOrNull(ctx context.Context, repoRoot, ref, file, destPath string) string {
	if err := gitShow(ctx, repoRoot, ref, file, destPath); err != nil {
		return os.DevNull
	}
	return destPath
}

// worktreeOrNull returns path if it exists on disk, else the null device — a
// deleted-but-tracked file has no worktree side, and difft exits 2 on a missing
// operand rather than treating it as an empty file.
func worktreeOrNull(path string) string {
	if _, err := os.Stat(path); err != nil {
		return os.DevNull
	}
	return path
}

func gitShow(ctx context.Context, repoRoot, ref, file, destPath string) error {
	cmd := exec.CommandContext(ctx, "git", "show", ref+":"+file)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	return writeTemp(destPath, out)
}
