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
	if file == "" && opts.Width > 0 {
		return d.diffAllDirect(ctx, repoRoot, opts)
	}
	if opts.Width <= 0 {
		return d.diffViaGit(ctx, repoRoot, file, opts)
	}
	return d.diffDirect(ctx, repoRoot, file, opts)
}

func (d *difftasticEngine) diffAllDirect(ctx context.Context, repoRoot string, opts DiffOpts) (string, error) {
	files, err := changedFileNames(ctx, repoRoot, opts)
	if err != nil || len(files) == 0 {
		return "", err
	}

	var result strings.Builder
	for _, file := range files {
		content, err := d.diffDirect(ctx, repoRoot, file, opts)
		if err != nil {
			continue
		}
		if content != "" {
			result.WriteString(content)
			result.WriteString("\n")
		}
	}
	return result.String(), nil
}

func changedFileNames(ctx context.Context, repoRoot string, opts DiffOpts) ([]string, error) {
	args := []string{"diff", "--name-only"}
	if opts.Staged {
		args = append(args, "--staged")
	} else if opts.Base != "" && opts.Target != "" {
		args = append(args, opts.Base, opts.Target)
	} else if opts.Base != "" {
		args = append(args, opts.Base)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return splitLines(string(out)), nil
		}
		return nil, fmt.Errorf("git diff --name-only: %w", err)
	}
	return splitLines(string(out)), nil
}

func splitLines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (d *difftasticEngine) diffViaGit(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error) {
	args := buildGitDiffArgs(opts, file)
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
	tmpDir, err := os.MkdirTemp("", "flux-diff-*")
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
		newPath = filepath.Join(repoRoot, file)
	default:
		newPath = filepath.Join(repoRoot, file)
	}

	oldPath := showOrNull(ctx, repoRoot, oldRef, file, filepath.Join(tmpDir, "a", file))
	return d.diffFiles(ctx, oldPath, newPath, opts.Color, opts.Width)
}

func (d *difftasticEngine) DiffCommit(ctx context.Context, repoRoot, base, target string, color bool, width int) (string, error) {
	// Get list of changed files
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", base+".."+target)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff --name-only: %w", err)
	}

	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(files) == 0 || (len(files) == 1 && files[0] == "") {
		return "", nil
	}

	tmpDir, err := os.MkdirTemp("", "flux-diff-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use subdirs so difft shows "a/file" vs "b/file" in its header
	aDir := filepath.Join(tmpDir, "a")
	bDir := filepath.Join(tmpDir, "b")

	var result strings.Builder
	for _, file := range files {
		oldPath := showOrNull(ctx, repoRoot, base, file, filepath.Join(aDir, file))
		newPath := showOrNull(ctx, repoRoot, target, file, filepath.Join(bDir, file))

		diffOut, err := d.diffFiles(ctx, oldPath, newPath, color, width)
		if err != nil {
			continue
		}
		if diffOut != "" {
			result.WriteString(diffOut)
			result.WriteString("\n")
		}
	}
	return result.String(), nil
}

// diffFiles calls difft directly in 2-arg mode. Note: difft ignores --width
// for pure additions (old=/dev/null) even in side-by-side mode. Callers should
// hard-wrap the output as a safety net. See https://github.com/Wilfred/difftastic/issues/861
func (d *difftasticEngine) diffFiles(ctx context.Context, oldPath, newPath string, color bool, width int) (string, error) {
	args := []string{"--display", "side-by-side"}
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

func showOrNull(ctx context.Context, repoRoot, ref, file, destPath string) string {
	if err := gitShow(ctx, repoRoot, ref, file, destPath); err != nil {
		return "/dev/null"
	}
	return destPath
}

func gitShow(ctx context.Context, repoRoot, ref, file, destPath string) error {
	cmd := exec.CommandContext(ctx, "git", "show", ref+":"+file)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0700); err != nil {
		return err
	}
	return os.WriteFile(destPath, out, 0600)
}
