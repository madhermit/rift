package cmd

import (
	"fmt"

	"github.com/madhermit/rift/internal/output"
	"github.com/spf13/cobra"
)

var Version = "dev"

type versionInfo struct {
	Version string `json:"version"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of rift",
	RunE: func(cmd *cobra.Command, args []string) error {
		w := cmd.OutOrStdout()
		if output.Detect(cmd) == output.JSON {
			return output.WriteJSON(w, versionInfo{Version: Version})
		}
		fmt.Fprintf(w, "rift version %s\n", Version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
