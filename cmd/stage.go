package cmd

import (
	"fmt"
	"os"

	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/output"
	"github.com/madhermit/rift/internal/tui"
	stageui "github.com/madhermit/rift/internal/tui/stage"
	"github.com/spf13/cobra"
)

var stageCmd = &cobra.Command{
	Use:   "stage [-- path...]",
	Short: "Interactive staging with diff preview",
	Long:  "Stage and unstage files and hunks with syntax-aware diff preview.",
	Args:  pathsOnly,
	RunE:  runStage,
}

func init() {
	rootCmd.AddCommand(stageCmd)
}

// pathsOnly rejects anything but a trailing `-- path...`: a bare argument can't
// name a ref here, and silently ignoring it would show more than was asked for.
func pathsOnly(cmd *cobra.Command, args []string) error {
	if at := cmd.ArgsLenAtDash(); at > 0 || (at < 0 && len(args) > 0) {
		return fmt.Errorf("unexpected argument %q: paths go after --", args[0])
	}
	return nil
}

func runStage(cmd *cobra.Command, args []string) error {
	mode := output.Detect(cmd)
	_, paths := splitAtDash(cmd, args)

	repo, err := git.OpenRepo()
	if err != nil {
		return err
	}

	files, err := repo.StatusFiles(paths...)
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
		engine := diff.NewEngine()
		m := stageui.New(repo, engine, files, paths)
		_, err := tui.NewProgram(m).Run()
		return err
	}
}
