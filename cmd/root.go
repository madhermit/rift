package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/madhermit/rift/internal/output"
	"github.com/madhermit/rift/internal/tui/menu"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "rift",
	Short:         "Syntax-aware, worktree-native, composable fuzzy git tool",
	Long:          "rift is a syntax-aware, worktree-native, composable fuzzy git tool.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.RunE = runRoot
	rootCmd.PersistentFlags().Bool("print", false, "Output in plain text (non-interactive)")
	rootCmd.PersistentFlags().Bool("json", false, "Output in JSON format")
	rootCmd.PersistentFlags().String("format", "", "Output format template")
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
	p := tea.NewProgram(m, tea.WithAltScreen())
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
