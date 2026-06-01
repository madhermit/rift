package diff

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/madhermit/rift/internal/tooling"
)

// Display selects difftastic's layout. Auto picks side-by-side only when the
// pane is wide enough, otherwise inline — side-by-side halves an already-narrow
// pane and difftastic doesn't reliably honor --width.
type Display int

const (
	DisplayAuto Display = iota
	DisplayInline
	DisplaySideBySide
)

// sideBySideMinWidth is the pane width (columns) below which Auto uses inline.
const sideBySideMinWidth = 120

// difftValue resolves the mode to difftastic's --display argument.
func (d Display) difftValue(width int) string {
	switch d {
	case DisplayInline:
		return "inline"
	case DisplaySideBySide:
		return "side-by-side"
	default:
		if width >= sideBySideMinWidth {
			return "side-by-side"
		}
		return "inline"
	}
}

// Next cycles Auto → Inline → SideBySide → Auto for the layout toggle.
func (d Display) Next() Display { return (d + 1) % 3 }

type DiffOpts struct {
	Staged  bool
	Base    string
	Target  string
	Color   bool
	Width   int
	Display Display
}

type Engine interface {
	Diff(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error)
	DiffCommit(ctx context.Context, repoRoot, base, target string, color bool, width int, display Display) (string, error)
	DiffHunks(ctx context.Context, hunks []Hunk, filename, baseContent string, color bool, width int) []string
	Name() string
}

func NewEngine() Engine {
	path, err := tooling.FindOrInstallDifft()
	if err == nil && path != "" {
		return &difftasticEngine{path: path}
	}
	return &fallbackEngine{}
}

// NewPlainEngine returns the plain git-diff engine — the non-structural
// alternative offered by the engine toggle.
func NewPlainEngine() Engine {
	return &fallbackEngine{}
}

// displayFlags add intra-line word coloring and whitespace-error highlighting to
// the plain git-diff engine's output. They apply only in color mode and only to
// the fallback engine — difftastic supplies its own structural intra-line
// highlighting, and the raw-diff path used for hunk parsing must stay vanilla.
func displayFlags(color bool) []string {
	if !color {
		return nil
	}
	return []string{"--word-diff=color", "--ws-error-highlight=all"}
}

func buildCommitDiffArgs(base, target string, color bool) []string {
	args := []string{"diff"}
	if color {
		args = append(args, "--color=always")
	} else {
		args = append(args, "--color=never")
	}
	args = append(args, displayFlags(color)...)
	args = append(args, base+".."+target)
	return args
}

// buildGitDiffArgs builds `git diff` arguments. display adds the fallback
// engine's readability flags; the difftastic-via-git path passes false so its
// external-diff driver sees a plain diff.
func buildGitDiffArgs(opts DiffOpts, file string, display bool) []string {
	args := []string{"diff"}
	if opts.Color {
		args = append(args, "--color=always")
	} else {
		args = append(args, "--color=never")
	}
	if display {
		args = append(args, displayFlags(opts.Color)...)
	}
	if opts.Staged {
		args = append(args, "--staged")
	} else if opts.Base != "" && opts.Target != "" {
		args = append(args, opts.Base, opts.Target)
	} else if opts.Base != "" {
		args = append(args, opts.Base)
	}
	if file != "" {
		args = append(args, "--", file)
	}
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
