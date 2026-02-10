package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/output"
	stageui "github.com/madhermit/rift/internal/tui/stage"
	"github.com/spf13/cobra"
)

var stageCmd = &cobra.Command{
	Use:   "stage [-- path...]",
	Short: "Interactive staging with diff preview",
	Long:  "Stage and unstage files and hunks with syntax-aware diff preview.",
	RunE:  runStage,
}

func init() {
	rootCmd.AddCommand(stageCmd)
}

func runStage(cmd *cobra.Command, args []string) error {
	mode := output.Detect(cmd)

	repo, err := git.OpenRepo()
	if err != nil {
		return err
	}

	files, err := repo.StatusFiles()
	if err != nil {
		return err
	}

	switch mode {
	case output.JSON:
		return output.WriteJSON(os.Stdout, files)
	case output.Print:
		lines := make([]string, len(files))
		for i, f := range files {
			lines[i] = fmt.Sprintf("%s%s %s", git.StatusChar(f.StagingStatus), git.StatusChar(f.WorktreeStatus), f.Path)
		}
		return output.WritePlain(os.Stdout, lines)
	default:
		if len(files) == 0 {
			fmt.Println("No changes found.")
			return nil
		}

		engine := diff.NewEngine()
		m := stageui.New(repo, engine, files)
		_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
		return err
	}
}
