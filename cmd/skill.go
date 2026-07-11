package cmd

import (
	"fmt"

	"github.com/madhermit/rift/internal/output"
	"github.com/madhermit/rift/internal/skill"
	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Print the agent skill that teaches rift's review workflow",
	Long: "Print the built-in agent skill: a document that teaches an AI agent how to review " +
		"changes with rift's composable --print/--json output. Point an agent at it with " +
		"`rift skill path`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if output.Detect(cmd) == output.JSON {
			return output.WriteJSON(cmd.OutOrStdout(), skillInfo{Content: skill.Content()})
		}
		fmt.Fprint(cmd.OutOrStdout(), skill.Content())
		return nil
	},
}

var skillPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Write the skill to its managed location and print the path",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := skill.Path()
		if err != nil {
			return err
		}
		if output.Detect(cmd) == output.JSON {
			return output.WriteJSON(cmd.OutOrStdout(), skillInfo{Path: p})
		}
		fmt.Fprintln(cmd.OutOrStdout(), p)
		return nil
	},
}

type skillInfo struct {
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

func init() {
	skillCmd.AddCommand(skillPathCmd)
	rootCmd.AddCommand(skillCmd)
}
