package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/madhermit/rift/internal/output"
	"github.com/madhermit/rift/internal/tui"
	"github.com/madhermit/rift/internal/tui/menu"
	"github.com/spf13/cobra"
)

// runGit runs a git write command with the terminal's stdio attached, so
// interactive operations (editor prompts, conflict output) work after the TUI
// exits.
func runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var rootCmd = &cobra.Command{
	Use:           "rift",
	Short:         "Syntax-aware, worktree-aware, composable fuzzy git tool",
	Long:          "rift is a syntax-aware, worktree-aware, composable fuzzy git tool.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.RunE = runRoot
	rootCmd.PersistentFlags().Bool("print", false, "Output in plain text (non-interactive)")
	rootCmd.PersistentFlags().Bool("json", false, "Output in JSON format")
}

func Execute() error {
	return rootCmd.Execute()
}

func runRoot(cmd *cobra.Command, args []string) error {
	mode := output.Detect(cmd)
	if mode != output.Interactive {
		return cmd.Help()
	}

	// The menu is a launchpad: after a chosen screen exits cleanly, return to the
	// menu instead of quitting rift. Quitting the menu itself (empty selection)
	// exits, an error breaks out, and a screen that ran a git write exits too so
	// git's output (e.g. a conflict message) stays on the terminal.
	for {
		selected, err := runMenu()
		if err != nil {
			return err
		}
		var actionRan bool
		if selected != "" {
			if actionRan, err = runMenuSelection(selected); err != nil {
				return err
			}
		}
		if menuLoopExits(selected, actionRan) {
			return nil
		}
	}
}

// menuLoopExits reports whether runRoot should stop after a menu selection: it
// stops when the user quit the menu (empty selection) or the chosen screen ran a
// git write (whose output must stay visible); otherwise it re-displays the menu.
func menuLoopExits(selected string, actionRan bool) bool {
	return selected == "" || actionRan
}

// runMenu shows the launchpad and returns the chosen command name, or "" when
// the user quit the menu.
func runMenu() (string, error) {
	result, err := tui.NewProgram(menu.New()).Run()
	if err != nil {
		return "", err
	}
	if final, ok := result.(menu.Model); ok {
		return final.Selected(), nil
	}
	return "", nil
}

// runMenuSelection runs the command the menu chose, reporting whether it
// performed a git write — the stash/log actions run after their TUI exits, so
// the loop must not re-enter the menu over git's output. diff and stage never
// write, so their RunE result carries no action.
func runMenuSelection(name string) (actionRan bool, err error) {
	sub, _, err := rootCmd.Find([]string{name})
	if err != nil {
		return false, fmt.Errorf("command %q not found: %w", name, err)
	}
	switch name {
	case "stash":
		return runStashAction(sub, nil)
	case "log":
		return runLogAction(sub, nil)
	default:
		return false, sub.RunE(sub, nil)
	}
}
