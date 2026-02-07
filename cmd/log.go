package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/madhermit/flux/internal/diff"
	"github.com/madhermit/flux/internal/git"
	"github.com/madhermit/flux/internal/output"
	logui "github.com/madhermit/flux/internal/tui/log"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log [flags] [ref] [-- path...]",
	Short: "Interactive commit log browser",
	Long:  "Browse commit history with syntax-aware diff preview. Supports fuzzy filtering and split-pane browsing.",
	RunE:  runLog,
}

func init() {
	logCmd.Flags().IntP("max-count", "n", 200, "Maximum number of commits to show (0 for unlimited)")
	logCmd.Flags().Bool("all", false, "Show commits from all branches")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	mode := output.Detect(cmd)
	maxCount, _ := cmd.Flags().GetInt("max-count")
	all, _ := cmd.Flags().GetBool("all")
	refArgs, pathArgs := splitAtDash(cmd, args)

	repo, err := git.OpenRepo()
	if err != nil {
		return err
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
		return err
	}

	switch mode {
	case output.JSON:
		return output.WriteJSON(os.Stdout, commits)
	case output.Print:
		lines := make([]string, len(commits))
		for i, c := range commits {
			lines[i] = fmt.Sprintf("%s %s", c.Hash, c.Message)
		}
		return output.WritePlain(os.Stdout, lines)
	default:
		engine := diff.NewEngine()
		m := logui.New(repo, engine, commits)
		_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
		return err
	}
}
