package cmd

import (
	"fmt"
	"os"

	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/output"
	"github.com/madhermit/rift/internal/tui"
	logui "github.com/madhermit/rift/internal/tui/log"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log [flags] [<ref> | <range>] [-- path...]",
	Short: "Interactive commit log browser",
	Long: "Browse commit history with syntax-aware diff preview. Supports fuzzy " +
		"filtering and split-pane browsing.\n\n" +
		"Accepts a ref (branch, tag, @{upstream}) or a commit range such as " +
		"main..HEAD or @{upstream}.. to scope the log.",
	RunE: runLog,
}

func init() {
	logCmd.Flags().IntP("max-count", "n", 200, "Maximum number of commits to show (0 for unlimited)")
	logCmd.Flags().Bool("all", false, "Show commits from all branches")
	logCmd.Flags().Bool("tests", false, "Drill into each commit's touched test cases instead of its files")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	_, err := runLogAction(cmd, args)
	return err
}

// runLogAction runs the log command and reports whether it performed a git write
// (cherry-pick/revert after the TUI exits). The menu loop uses that flag to
// decide between re-entering the menu (nothing ran) and exiting so git's output
// — e.g. a conflict message — stays on the terminal.
func runLogAction(cmd *cobra.Command, args []string) (actionRan bool, err error) {
	mode := output.Detect(cmd)
	maxCount, _ := cmd.Flags().GetInt("max-count")
	all, _ := cmd.Flags().GetBool("all")
	refArgs, pathArgs := splitAtDash(cmd, args)

	// A single optional ref scopes the log; a second ref would be silently
	// dropped, so reject it (mirrors DiffTargets rejecting a third ref).
	if len(refArgs) > 1 {
		return false, fmt.Errorf("too many arguments: expected at most 1 commit ref")
	}

	repo, err := git.OpenRepo()
	if err != nil {
		return false, err
	}

	var commits []git.CommitInfo
	if all {
		commits, err = repo.LogAll(maxCount, pathArgs)
	} else {
		ref := "HEAD"
		if len(refArgs) > 0 {
			ref = refArgs[0]
		}
		commits, err = repo.Log(ref, maxCount, pathArgs)
	}
	if err != nil {
		return false, err
	}

	switch mode {
	case output.JSON:
		return false, output.WriteJSON(os.Stdout, commits)
	case output.Print:
		lines := make([]string, len(commits))
		for i, c := range commits {
			lines[i] = fmt.Sprintf("%s %s", c.Hash, c.Message)
		}
		return false, output.WritePlain(os.Stdout, lines)
	default:
		engine := diff.NewEngine()
		tests, _ := cmd.Flags().GetBool("tests")
		m := logui.New(repo, engine, commits, tests)
		result, err := tui.NewProgram(m).Run()
		if err != nil {
			return false, err
		}
		final, ok := result.(logui.Model)
		if !ok {
			return false, nil
		}
		hash := final.SelectedHash()
		if hash == "" {
			return false, nil
		}
		var verb string
		switch final.Action() {
		case logui.CherryPick:
			verb = "cherry-pick"
		case logui.Revert:
			verb = "revert"
		default:
			return false, nil
		}
		return true, runGit(verb, hash)
	}
}
