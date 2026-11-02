package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	// `git stash list` exits 0 with empty output when there are no stashes, so a
	// non-zero status is a real failure — surface its stderr rather than masking
	// it as "no stashes".
	out, err := r.runGit("stash", "list", "--format=%gd%x00%gs%x00%ci")
	if err != nil {
		return nil, err
	}

	text := strings.TrimSpace(out)
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

// StashRange is the pair of tree-ishes whose diff is everything a stash holds. A
// stash's commit only records tracked changes; `stash -u` keeps untracked files
// in a third parent, which a plain ref^..ref diff would never show. When there is
// one, the target is a tree overlaying those files on the stash's own, so they
// read as additions and a reader sees what popping would bring back. If building
// that tree fails it degrades to the tracked changes.
func (r *Repo) StashRange(index int) (base, target string) {
	ref := fmt.Sprintf("stash@{%d}", index)
	base = ref + "^"
	if _, err := r.runGit("rev-parse", "--verify", "--quiet", ref+"^3"); err != nil {
		return base, ref
	}
	tree, err := r.overlayTree(ref, ref+"^3")
	if err != nil {
		return base, ref
	}
	return base, tree
}

// overlayTree writes a tree holding top's files laid over base's, using a
// throwaway index so the real one is never touched. The only trace it leaves is
// an unreferenced object, which git prunes.
func (r *Repo) overlayTree(base, top string) (string, error) {
	dir, err := os.MkdirTemp("", "rift-stash-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	env := append(os.Environ(), "GIT_INDEX_FILE="+filepath.Join(dir, "index"))
	run := func(stdin string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env, cmd.Stdin = r.root, env, strings.NewReader(stdin)
		out, err := cmd.Output()
		if err != nil {
			return "", gitErr("git "+strings.Join(args, " "), err)
		}
		return string(out), nil
	}
	if _, err := run("", "read-tree", base); err != nil {
		return "", err
	}
	listing, err := r.runGit("ls-tree", "-r", "-z", top)
	if err != nil {
		return "", err
	}
	if _, err := run(listing, "update-index", "-z", "--index-info"); err != nil {
		return "", err
	}
	tree, err := run("", "write-tree")
	return strings.TrimSpace(tree), err
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
