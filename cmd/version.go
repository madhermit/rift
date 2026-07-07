package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/madhermit/rift/internal/output"
	"github.com/spf13/cobra"
)

// Version is set via -ldflags on release/mise builds. Plain `go install`
// leaves it "dev"; resolveVersion then falls back to the module version so
// go-install builds still self-identify.
var Version = "dev"

type versionInfo struct {
	Version string `json:"version"`
}

// resolveVersion returns the ldflags Version, or the module version from the
// build info when Version was never stamped (a plain `go install ...@latest`).
func resolveVersion() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return Version
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of rift",
	RunE: func(cmd *cobra.Command, args []string) error {
		w := cmd.OutOrStdout()
		v := resolveVersion()
		if output.Detect(cmd) == output.JSON {
			return output.WriteJSON(w, versionInfo{Version: v})
		}
		fmt.Fprintf(w, "rift version %s\n", v)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
