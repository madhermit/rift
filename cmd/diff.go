package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/madhermit/flux/internal/diff"
	"github.com/madhermit/flux/internal/git"
	"github.com/madhermit/flux/internal/output"
	diffui "github.com/madhermit/flux/internal/tui/diff"
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

	files, err := listChangedFiles(repo, staged, base, target)
	if err != nil {
		return err
	}
	files = git.FilterByPaths(files, pathArgs)

	if nameOnly {
		return printFileNames(files)
	}

	switch mode {
	case output.JSON:
		return output.WriteJSON(os.Stdout, files)
	case output.Print:
		return printDiffs(engine, repo, files, staged, base, target)
	default:
		m := diffui.New(repo, engine, files, staged, base, target)
		_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
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
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
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
