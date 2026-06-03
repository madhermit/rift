package cmd

import (
	"fmt"
	"os"

	"github.com/madhermit/rift/internal/git"
	"github.com/madhermit/rift/internal/output"
	"github.com/madhermit/rift/internal/review"
)

// runTestsLens renders the test cases a diff touches as a plain listing or JSON.
// The interactive view lives in the lens wrapper (internal/tui/lens), which
// toggles between this and the file view.
func runTestsLens(mode output.Mode, repo *git.Repo, scope review.DiffScope) error {
	specs, err := review.Collect(repo, scope)
	if err != nil {
		return err
	}
	if mode == output.JSON {
		if specs == nil {
			specs = []review.Spec{}
		}
		return output.WriteJSON(os.Stdout, specs)
	}
	return output.WritePlain(os.Stdout, renderSpecs(specs))
}

// renderSpecs groups specs by file into a plain indented listing for --print.
func renderSpecs(specs []review.Spec) []string {
	if len(specs) == 0 {
		return []string{"No test changes in scope."}
	}

	var lines []string
	counts := map[string]int{}
	lastFile := ""
	for _, s := range specs {
		if s.File != lastFile {
			if lastFile != "" {
				lines = append(lines, "")
			}
			lines = append(lines, s.File)
			lastFile = s.File
		}
		counts[s.Status]++

		row := fmt.Sprintf("  %s %s%s L%d", s.Glyph(), s.PathPrefix(), s.Name, s.Line)
		if s.Ticket != "" {
			row += " " + s.Ticket
		}
		lines = append(lines, row)
	}

	lines = append(lines, "", fmt.Sprintf("%d tests · %d added · %d renamed · %d modified",
		len(specs), counts["added"], counts["renamed"], counts["modified"]))
	return lines
}
