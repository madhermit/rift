package diff

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/madhermit/rift/internal/tooling"
)

// ParallelStream runs work(0..n-1) concurrently, bounded to the CPU count, and
// returns a channel yielding each result in index order, closed when done. A
// changeset's files are diffed in parallel — difftastic is CPU-bound at ~1s per
// large file — while a progressive consumer gets each file as soon as the ones
// before it are ready. The channel is buffered to n so the workers never block
// if the consumer stops reading early (e.g. the user navigated away).
func ParallelStream(n int, work func(i int) string) <-chan string {
	out := make(chan string, max(n, 0))
	go func() {
		defer close(out)
		results := make([]chan string, n)
		for i := range results {
			results[i] = make(chan string, 1)
		}
		limit := runtime.NumCPU()
		if limit > n {
			limit = n
		}
		if limit < 1 {
			limit = 1
		}
		sem := make(chan struct{}, limit)
		for i := 0; i < n; i++ {
			go func(i int) {
				sem <- struct{}{}
				defer func() { <-sem }()
				results[i] <- work(i)
			}(i)
		}
		for i := 0; i < n; i++ {
			out <- <-results[i]
		}
	}()
	return out
}

// gitErr wraps a shelled-git failure with the command's stderr. cmd.Output()
// captures stderr into ExitError.Stderr, so an opaque "exit status 128" carries
// git's actual fatal message (e.g. a bad revision or wrong work tree).
func gitErr(label string, err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			return fmt.Errorf("%s: %w: %s", label, err, msg)
		}
	}
	return fmt.Errorf("%s: %w", label, err)
}

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
	OldPath string // pre-image path when the file was renamed in this scope
	Color   bool
	Width   int
	Display Display
}

type Engine interface {
	Diff(ctx context.Context, repoRoot, file string, opts DiffOpts) (string, error)
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

// NewMovedEngine returns the git-diff engine with moved-line detection: git
// classifies relocated lines and colors them distinctly, answering "moved or
// rewritten?" at a glance. A separate engine (not a flag on the plain one)
// because git ignores --color-moved under --word-diff, so the two views are
// mutually exclusive.
func NewMovedEngine() Engine {
	return &fallbackEngine{moved: true}
}

// displayFlags are the git-diff engines' readability flags, color mode only —
// difftastic supplies its own structural intra-line highlighting, and the
// raw-diff path used for hunk parsing must stay vanilla. The default view adds
// intra-line word coloring; the moved view trades that for git's moved-line
// classification (zebra alternates colors between distinct moved blocks, and
// the ws mode keeps re-indented moves classified as moves).
func displayFlags(color, moved bool) []string {
	if !color {
		return nil
	}
	if moved {
		return []string{"--color-moved=zebra", "--color-moved-ws=allow-indentation-change", "--ws-error-highlight=all"}
	}
	return []string{"--word-diff=color", "--ws-error-highlight=all"}
}

// buildGitDiffArgs builds `git diff` arguments with the given extra display
// flags (nil for the difftastic-via-git path, whose external-diff driver must
// see a plain diff).
func buildGitDiffArgs(opts DiffOpts, file string, display []string) []string {
	args := []string{"diff"}
	if opts.Color {
		args = append(args, "--color=always")
	} else {
		args = append(args, "--color=never")
	}
	args = append(args, display...)
	if opts.Staged {
		args = append(args, "--staged")
	} else if opts.Base != "" && opts.Target != "" {
		args = append(args, opts.Base, opts.Target)
	} else if opts.Base != "" {
		args = append(args, opts.Base)
	}
	if file != "" {
		if opts.OldPath != "" && opts.OldPath != file {
			// A renamed file needs both sides in the pathspec for git to pair
			// them, and --find-renames guards against diff.renames=false config.
			return append(args, "--find-renames", "--", file, opts.OldPath)
		}
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
		return "", gitErr(label, err)
	}
	return string(out), nil
}
