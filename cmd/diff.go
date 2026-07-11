package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/output"
	"github.com/madhermit/rift/internal/review"
	"github.com/madhermit/rift/internal/tui"
	lensui "github.com/madhermit/rift/internal/tui/lens"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff [flags] [commit [commit]] [-- path...]",
	Short: "Browse changes with syntax-aware diffs",
	Long:  "Show file changes with syntax-aware diffing powered by difftastic. Supports fuzzy file filtering and split-pane browsing.",
	RunE:  runDiff,
}

func init() {
	diffCmd.Flags().Bool("staged", false, "Show staged changes")
	diffCmd.Flags().Bool("name-only", false, "Only show changed file names")
	diffCmd.Flags().Bool("tests", false, "Show the test cases the diff touches instead of files")
	diffCmd.Flags().Bool("unreviewed", false, "Only show files not yet marked reviewed")
	diffCmd.Flags().Bool("watch", false, "Reload automatically as the working tree changes")
	rootCmd.AddCommand(diffCmd)
}

func splitAtDash(cmd *cobra.Command, args []string) (refArgs, pathArgs []string) {
	if i := cmd.ArgsLenAtDash(); i >= 0 {
		return args[:i], args[i:]
	}
	return args, nil
}

func runDiff(cmd *cobra.Command, args []string) error {
	mode := output.Detect(cmd)
	staged, _ := cmd.Flags().GetBool("staged")
	nameOnly, _ := cmd.Flags().GetBool("name-only")
	refArgs, pathArgs := splitAtDash(cmd, args)

	repo, err := git.OpenRepo()
	if err != nil {
		return err
	}

	engine := diff.NewEngine()
	base, target, err := git.DiffTargets(refArgs)
	if err != nil {
		return err
	}

	// --staged compares the index against HEAD; a commit argument names a
	// different base entirely. The file list and the per-file diffs would then
	// disagree (the list honors the ref, each diff honors --staged), so reject
	// the combination rather than render mismatched content.
	if staged && base != "" {
		return fmt.Errorf("--staged cannot be combined with a commit argument")
	}

	// --watch reloads the TUI as the working tree changes: a committed range is
	// immutable, and a non-interactive listing has nothing to reload into.
	watch, _ := cmd.Flags().GetBool("watch")
	if watch && target != "" {
		return fmt.Errorf("--watch requires a working-tree diff, not a commit range")
	}
	if watch && (mode != output.Interactive || nameOnly) {
		return fmt.Errorf("--watch requires the interactive TUI")
	}

	// Committed() keys on Target, so a working-tree scope ignores Base and a
	// committed range ignores Staged — one literal covers both.
	scope := review.DiffScope{Staged: staged, Base: base, Target: target, Paths: pathArgs}
	tests, _ := cmd.Flags().GetBool("tests")

	// The interactive view is a lens that toggles files↔tests, so the tests flag
	// just chooses which side it opens on. --print/--json render the tests list
	// directly (no toggle to offer). --name-only is a file-name output, so it
	// takes precedence over --tests regardless of TTY.
	if tests && !nameOnly && mode != output.Interactive {
		return runTestsLens(mode, repo, scope)
	}

	files, err := repo.ListChanged(staged, base, target)
	if err != nil {
		return err
	}
	files = git.FilterByPaths(files, pathArgs)

	// --unreviewed narrows any non-interactive listing to files not yet marked
	// reviewed (a working-tree concept). --name-only prints a plain listing even
	// on a TTY, so gate on "emitting a listing" (non-interactive mode or
	// --name-only) to keep the piped and on-TTY file sets identical. The
	// interactive view instead keeps the full set and opens with the same live
	// filter the `U` key toggles, so it can be switched off without restarting.
	unrev, _ := cmd.Flags().GetBool("unreviewed")
	unrev = unrev && target == ""
	if unrev && (mode != output.Interactive || nameOnly) {
		files = filterUnreviewed(repo, files)
	}

	if nameOnly {
		return printFileNames(files)
	}

	switch mode {
	case output.JSON:
		return output.WriteJSON(os.Stdout, files)
	case output.Print:
		return printDiffs(engine, repo, files, staged, base, target)
	default:
		m := lensui.New(repo, engine, files, scope, tests, unrev, watch)
		_, err := tui.NewProgram(m).Run()
		return err
	}
}

// filterUnreviewed drops files whose current content is marked reviewed.
func filterUnreviewed(repo *git.Repo, files []git.ChangedFile) []git.ChangedFile {
	return review.LoadReviewed(repo).Unreviewed(files, review.ContentHashes(repo, files))
}

func printFileNames(files []git.ChangedFile) error {
	lines := make([]string, len(files))
	for i, f := range files {
		lines[i] = f.Path
	}
	return output.WritePlain(os.Stdout, lines)
}

func printDiffs(engine diff.Engine, repo *git.Repo, files []git.ChangedFile, staged bool, base, target string) error {
	ctx := context.Background()
	for _, f := range files {
		out, err := fileDiff(ctx, engine, repo, f, staged, base, target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: diff failed for %s: %v\n", f.Path, err)
			continue
		}
		if out != "" {
			fmt.Fprint(os.Stdout, out)
		}
	}
	return nil
}

// fileDiff renders one file's diff for --print. An untracked file has no tracked
// counterpart, so `git diff -- file` yields nothing; diff it against /dev/null
// instead so its content shows (the same route internal/tui/stage takes).
func fileDiff(ctx context.Context, engine diff.Engine, repo *git.Repo, f git.ChangedFile, staged bool, base, target string) (string, error) {
	if f.Status == "Untracked" {
		return diff.RawNewFileDiff(repo.Root(), f.Path)
	}
	return engine.Diff(ctx, repo.Root(), f.Path, diff.DiffOpts{
		Staged: staged,
		Base:   base,
		Target: target,
		Color:  false,
	})
}
