package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type StashEntry struct {
	Index   int    `json:"index"`
	Branch  string `json:"branch"`
	Message string `json:"message"`
	Date    string `json:"date"`
}

func (r *Repo) ListStashes() ([]StashEntry, error) {
	cmd := exec.Command("git", "-C", r.root, "stash", "list", "--format=%gd%x00%gs%x00%ci")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []StashEntry{}, nil
		}
		return nil, fmt.Errorf("git stash list: %w", err)
	}

	text := strings.TrimSpace(string(out))
	if text == "" {
		return []StashEntry{}, nil
	}

	entries := []StashEntry{}
	for _, line := range strings.Split(text, "\n") {
		parts := strings.SplitN(line, "\x00", 3)
		if len(parts) < 3 {
			continue
		}
		ref, subject, date := parts[0], parts[1], parts[2]

		index := parseStashIndex(ref)
		branch := parseStashBranch(subject)
		message := parseStashMessage(subject)

		entries = append(entries, StashEntry{
			Index:   index,
			Branch:  branch,
			Message: message,
			Date:    date,
		})
	}

	return entries, nil
}

// parseStashIndex extracts N from "stash@{N}".
func parseStashIndex(ref string) int {
	start := strings.Index(ref, "{")
	end := strings.Index(ref, "}")
	if start < 0 || end < 0 || end <= start+1 {
		return 0
	}
	n, _ := strconv.Atoi(ref[start+1 : end])
	return n
}

// parseStashBranch extracts the branch name from stash subject lines like
// "WIP on main: abc1234 msg" or "On main: abc1234 msg".
func parseStashBranch(subject string) string {
	for _, prefix := range []string{"WIP on ", "On "} {
		if strings.HasPrefix(subject, prefix) {
			rest := subject[len(prefix):]
			if idx := strings.Index(rest, ":"); idx >= 0 {
				return rest[:idx]
			}
		}
	}
	return ""
}

// parseStashMessage extracts the message portion after "branch: " from subject.
func parseStashMessage(subject string) string {
	if idx := strings.Index(subject, ": "); idx >= 0 {
		return subject[idx+2:]
	}
	return subject
}
