package cmd

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/output"
	stashui "github.com/madhermit/rift/internal/tui/stash"
	"github.com/spf13/cobra"
)

var stashCmd = &cobra.Command{
	Use:   "stash",
	Short: "Stash manager with preview",
	Long:  "Browse and manage stashes with syntax-aware diff preview.",
	RunE:  runStash,
}

func init() {
	rootCmd.AddCommand(stashCmd)
}

func runStash(cmd *cobra.Command, args []string) error {
	mode := output.Detect(cmd)

	repo, err := git.OpenRepo()
	if err != nil {
		return err
	}

	stashes, err := repo.ListStashes()
	if err != nil {
		return err
	}

	switch mode {
	case output.JSON:
		return output.WriteJSON(os.Stdout, stashes)
	case output.Print:
		lines := make([]string, len(stashes))
		for i, s := range stashes {
			lines[i] = fmt.Sprintf("stash@{%d} %s", s.Index, s.Message)
		}
		return output.WritePlain(os.Stdout, lines)
	default:
		if len(stashes) == 0 {
			fmt.Println("No stashes found.")
			return nil
		}

		engine := diff.NewEngine()
		m := stashui.New(repo, engine, stashes)
		result, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
		if err != nil {
			return err
		}

		if final, ok := result.(stashui.Model); ok {
			idx := final.SelectedIndex()
			if idx < 0 {
				return nil
			}
			switch final.Action() {
			case stashui.Apply:
				return gitStashAction("apply", idx)
			case stashui.Pop:
				return gitStashAction("pop", idx)
			case stashui.Drop:
				return gitStashAction("drop", idx)
			}
		}
		return nil
	}
}

func gitStashAction(action string, index int) error {
	gitCmd := exec.Command("git", "stash", action, fmt.Sprintf("stash@{%d}", index))
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	return gitCmd.Run()
}
