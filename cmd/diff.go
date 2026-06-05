package cmd

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/output"
	"github.com/madhermit/rift/internal/review"
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

	files, err := listChangedFiles(repo, staged, base, target)
	if err != nil {
		return err
	}
	files = git.FilterByPaths(files, pathArgs)

	// --unreviewed narrows the non-interactive output to files not yet marked
	// reviewed (a working-tree concept). The interactive view keeps the full set
	// and toggles the same filter live with `u`.
	if unrev, _ := cmd.Flags().GetBool("unreviewed"); unrev && target == "" && mode != output.Interactive {
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
		m := lensui.New(repo, engine, files, scope, tests)
		_, err := tea.NewProgram(m).Run()
		return err
	}
}

func listChangedFiles(repo *git.Repo, staged bool, base, target string) ([]git.ChangedFile, error) {
	var (
		files []git.ChangedFile
		err   error
	)
	if target != "" {
		files, err = repo.DiffBetweenCommits(base, target)
	} else {
		files, err = repo.ChangedFiles(staged)
	}
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []git.ChangedFile{}
	}
	git.SortByPath(files)
	return files, nil
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
		out, err := engine.Diff(ctx, repo.Root(), f.Path, diff.DiffOpts{
			Staged: staged,
			Base:   base,
			Target: target,
			Color:  false,
		})
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
