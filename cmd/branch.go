package cmd

import (
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/madhermit/flux/internal/git"
	"github.com/madhermit/flux/internal/output"
	branchui "github.com/madhermit/flux/internal/tui/branch"
	"github.com/spf13/cobra"
)

var branchCmd = &cobra.Command{
	Use:   "branch",
	Short: "Fuzzy branch switcher",
	Long:  "Browse and switch branches with fuzzy filtering.",
	RunE:  runBranch,
}

func init() {
	rootCmd.AddCommand(branchCmd)
}

func runBranch(cmd *cobra.Command, args []string) error {
	mode := output.Detect(cmd)

	repo, err := git.OpenRepo()
	if err != nil {
		return err
	}

	branches, err := repo.ListBranches()
	if err != nil {
		return err
	}

	switch mode {
	case output.JSON:
		return output.WriteJSON(os.Stdout, branches)
	case output.Print:
		lines := make([]string, len(branches))
		for i, b := range branches {
			prefix := "  "
			if b.Current {
				prefix = "* "
			}
			lines[i] = prefix + b.Name
		}
		return output.WritePlain(os.Stdout, lines)
	default:
		m := branchui.New(branches)
		result, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
		if err != nil {
			return err
		}

		if final, ok := result.(branchui.Model); ok {
			name := final.Checkout()
			if name != "" {
				return gitCheckout(name)
			}
		}
		return nil
	}
}

func gitCheckout(branch string) error {
	gitCmd := exec.Command("git", "checkout", branch)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	return gitCmd.Run()
}
