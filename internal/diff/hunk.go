package diff

import (
	"os/exec"
	"strconv"
	"strings"
)

type Hunk struct {
	Header   string
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []string
}

type FileDiff struct {
	Header string
	Path   string
	Hunks  []Hunk
}

func ParseUnifiedDiff(raw string) []FileDiff {
	if raw == "" {
		return nil
	}

	// Split into file sections on "diff --git" boundaries.
	var sections []string
	lines := strings.Split(raw, "\n")
	current := -1
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			sections = append(sections, "")
			current++
		}
		if current < 0 {
			continue
		}
		sections[current] += line + "\n"
	}

	var result []FileDiff
	for _, section := range sections {
		fd := parseFileSection(section)
		if fd.Path != "" {
			result = append(result, fd)
		}
	}
	return result
}

func parseFileSection(section string) FileDiff {
	lines := strings.Split(strings.TrimRight(section, "\n"), "\n")
	if len(lines) == 0 {
		return FileDiff{}
	}

	var fd FileDiff
	var headerEnd int
	for i, line := range lines {
		if strings.HasPrefix(line, "@@") {
			headerEnd = i
			break
		}
		if i == len(lines)-1 {
			// No hunks (binary file, etc.)
			headerEnd = len(lines)
		}
	}

	fd.Header = strings.Join(lines[:headerEnd], "\n") + "\n"
	fd.Path = extractPath(lines)

	// Parse hunks
	var currentHunk *Hunk
	for _, line := range lines[headerEnd:] {
		if strings.HasPrefix(line, "@@") {
			if currentHunk != nil {
				fd.Hunks = append(fd.Hunks, *currentHunk)
			}
			h := parseHunkHeader(line)
			currentHunk = &h
		} else if currentHunk != nil {
			currentHunk.Lines = append(currentHunk.Lines, line)
		}
	}
	if currentHunk != nil {
		fd.Hunks = append(fd.Hunks, *currentHunk)
	}

	return fd
}

func extractPath(lines []string) string {
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ b/") {
			return line[6:]
		}
		if strings.HasPrefix(line, "+++ /dev/null") {
			// Deleted file — use the "---" line
			for _, l := range lines {
				if strings.HasPrefix(l, "--- a/") {
					return l[6:]
				}
			}
		}
	}
	// Fallback: parse from "diff --git a/X b/X"
	if len(lines) > 0 && strings.HasPrefix(lines[0], "diff --git a/") {
		parts := strings.SplitN(lines[0], " b/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return ""
}

func parseHunkHeader(line string) Hunk {
	h := Hunk{Header: line}
	// Parse @@ -old,count +new,count @@
	rest := strings.TrimPrefix(line, "@@ ")
	end := strings.Index(rest, " @@")
	if end < 0 {
		return h
	}
	ranges := rest[:end]
	parts := strings.SplitN(ranges, " ", 2)
	if len(parts) != 2 {
		return h
	}
	h.OldStart, h.OldCount = parseRange(strings.TrimPrefix(parts[0], "-"))
	h.NewStart, h.NewCount = parseRange(strings.TrimPrefix(parts[1], "+"))
	return h
}

func parseRange(s string) (int, int) {
	parts := strings.SplitN(s, ",", 2)
	start, _ := strconv.Atoi(parts[0])
	count := 1
	if len(parts) == 2 {
		count, _ = strconv.Atoi(parts[1])
	}
	return start, count
}

// ApplyHunk applies a single hunk to the base file content, producing a new
// file with only that hunk's changes. This gives difftastic a full file for
// tree-sitter parsing.
func ApplyHunk(base string, h Hunk) string {
	lines := strings.Split(base, "\n")

	// Extract new lines from hunk (context + additions)
	var newLines []string
	for _, line := range h.Lines {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case ' ', '+':
			newLines = append(newLines, line[1:])
		}
	}

	// Replace the hunk region (1-based OldStart)
	start := h.OldStart - 1
	if start < 0 {
		start = 0
	}
	end := start + h.OldCount
	if end > len(lines) {
		end = len(lines)
	}

	result := make([]string, 0, start+len(newLines)+(len(lines)-end))
	result = append(result, lines[:start]...)
	result = append(result, newLines...)
	result = append(result, lines[end:]...)
	return strings.Join(result, "\n")
}

// BaseContent retrieves the base file content for diffing.
// For unstaged diffs: the index version (git show :file).
// For staged diffs: the HEAD version (git show HEAD:file).
func BaseContent(repoRoot string, staged bool, file string) (string, error) {
	ref := "" // index version
	if staged {
		ref = "HEAD"
	}
	out, err := ShowFile(repoRoot, ref, file)
	return string(out), err
}

// ShowFile returns the content of file at a git ref (git show ref:file); an empty
// ref reads the staged/index version. It runs in repoRoot so linked-worktree
// layouts resolve.
func ShowFile(repoRoot, ref, file string) ([]byte, error) {
	cmd := exec.Command("git", "show", ref+":"+file)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err // never hand back partial stdout as if it were the file
	}
	return out, nil
}

// RawRangeDiff returns the unified diff of file between two commits (base..target).
func RawRangeDiff(repoRoot, base, target, file string) (string, error) {
	cmd := exec.Command("git", "diff", "--no-color", base, target, "--", file)
	cmd.Dir = repoRoot
	return runGitDiff(cmd, "git diff range")
}

func (h Hunk) Patch(fileHeader string) string {
	var b strings.Builder
	b.WriteString(fileHeader)
	b.WriteString(h.Header)
	b.WriteString("\n")
	for _, line := range h.Lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func RawUnifiedDiff(repoRoot string, staged bool, file string) (string, error) {
	args := []string{"diff", "--no-color"}
	if staged {
		args = append(args, "--staged")
	}
	args = append(args, "--", file)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	return runGitDiff(cmd, "git diff raw")
}

// RawNewFileDiff generates a unified diff for an untracked file by comparing
// /dev/null against the file.
func RawNewFileDiff(repoRoot, file string) (string, error) {
	cmd := exec.Command("git", "diff", "--no-color", "--no-index", "--", "/dev/null", file)
	cmd.Dir = repoRoot
	return runGitDiff(cmd, "git diff --no-index")
}

// FirstHunkLine returns the new-file line of the first hunk in file's diff, so an
// editor can open at the change rather than the top of the file. Returns 0 (no
// specific line) when the diff can't be produced or parsed.
func FirstHunkLine(repoRoot string, staged bool, file string) int {
	raw, err := RawUnifiedDiff(repoRoot, staged, file)
	if err != nil {
		return 0
	}
	files := ParseUnifiedDiff(raw)
	if len(files) > 0 && len(files[0].Hunks) > 0 {
		return files[0].Hunks[0].NewStart
	}
	return 0
}
