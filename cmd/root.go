package cmd

import (
	"fmt"
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/output"
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

	m := menu.New()
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return err
	}

	if final, ok := result.(menu.Model); ok {
		selected := final.Selected()
		if selected != "" {
			sub, _, err := rootCmd.Find([]string{selected})
			if err != nil {
				return fmt.Errorf("command %q not found: %w", selected, err)
			}
			return sub.RunE(sub, nil)
		}
	}

	return nil
}
