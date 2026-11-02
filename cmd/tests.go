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
func runTestsLens(mode output.Mode, repo *git.Repo, scope review.DiffScope, unreviewed bool) error {
	specs, err := review.Collect(repo, scope)
	if err != nil {
		return err
	}
	if unreviewed {
		if specs, err = dropReviewedSpecs(repo, scope, specs); err != nil {
			return err
		}
	}
	if mode == output.JSON {
		if specs == nil {
			specs = []review.Spec{}
		}
		return output.WriteJSON(os.Stdout, specs)
	}
	return output.WritePlain(os.Stdout, renderSpecs(specs))
}

// dropReviewedSpecs keeps the specs in files not yet marked reviewed, the same set
// --unreviewed narrows a file listing to.
func dropReviewedSpecs(repo *git.Repo, scope review.DiffScope, specs []review.Spec) ([]review.Spec, error) {
	files, err := repo.ListChanged(scope.Staged, scope.Base, scope.Target, scope.Paths...)
	if err != nil {
		return nil, err
	}
	open := map[string]bool{}
	for _, f := range filterUnreviewed(repo, files) {
		open[f.Path] = true
	}
	var kept []review.Spec
	for _, s := range specs {
		if open[s.File] {
			kept = append(kept, s)
		}
	}
	return kept, nil
}

// commitTests is a commit with the test cases it touched, for `log --tests --json`.
type commitTests struct {
	git.CommitInfo
	Tests []review.Spec `json:"tests"`
}

// writeCommitTests renders each commit's touched tests. JSON keeps every commit —
// a change that touches no tests is itself a finding, and `tests` being empty says
// so — while the plain listing shows only the commits that have something to list.
func writeCommitTests(mode output.Mode, repo *git.Repo, commits []git.CommitInfo, paths []string) error {
	results := make([]commitTests, 0, len(commits))
	for _, c := range commits {
		base, _, err := repo.CommitFiles(c.Hash)
		if err != nil {
			return err
		}
		specs, err := review.Collect(repo, review.DiffScope{Base: base, Target: c.Hash, Paths: paths})
		if err != nil {
			return err
		}
		if specs == nil {
			specs = []review.Spec{}
		}
		results = append(results, commitTests{CommitInfo: c, Tests: specs})
	}
	if mode == output.JSON {
		return output.WriteJSON(os.Stdout, results)
	}
	var lines []string
	for _, r := range results {
		if len(r.Tests) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, r.Hash+" "+r.Message)
		lines = append(lines, renderSpecs(r.Tests)...)
	}
	if len(lines) == 0 {
		lines = renderSpecs(nil)
	}
	return output.WritePlain(os.Stdout, lines)
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
