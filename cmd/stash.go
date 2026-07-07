package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/output"
	"github.com/madhermit/rift/internal/tui"
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
	_, err := runStashAction(cmd, args)
	return err
}

// runStashAction runs the stash command and reports whether it performed a git
// write (apply/pop/drop after the TUI exits). The menu loop uses that flag to
// decide between re-entering the menu (nothing ran) and exiting so git's output
// — e.g. a merge-conflict message — stays on the terminal.
func runStashAction(cmd *cobra.Command, args []string) (actionRan bool, err error) {
	mode := output.Detect(cmd)

	repo, err := git.OpenRepo()
	if err != nil {
		return false, err
	}

	stashes, err := repo.ListStashes()
	if err != nil {
		return false, err
	}

	switch mode {
	case output.JSON:
		return false, output.WriteJSON(os.Stdout, stashes)
	case output.Print:
		lines := make([]string, len(stashes))
		for i, s := range stashes {
			lines[i] = fmt.Sprintf("stash@{%d} %s", s.Index, s.Message)
		}
		return false, output.WritePlain(os.Stdout, lines)
	default:
		if len(stashes) == 0 {
			fmt.Println("No stashes found.")
			return false, nil
		}

		engine := diff.NewEngine()
		m := stashui.New(repo, engine, stashes)
		result, err := tui.NewProgram(m).Run()
		if err != nil {
			return false, err
		}

		final, ok := result.(stashui.Model)
		if !ok {
			return false, nil
		}
		idx := final.SelectedIndex()
		if idx < 0 {
			return false, nil
		}
		var verb string
		switch final.Action() {
		case stashui.Apply:
			verb = "apply"
		case stashui.Pop:
			verb = "pop"
		case stashui.Drop:
			verb = "drop"
		default:
			return false, nil
		}
		return true, gitStashAction(verb, idx)
	}
}

func gitStashAction(action string, index int) error {
	gitCmd := exec.Command("git", "stash", action, fmt.Sprintf("stash@{%d}", index))
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	return gitCmd.Run()
}
