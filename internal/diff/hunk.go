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

	// Split into file sections on "diff --git" boundaries, indexing into the
	// single split rather than concatenating per line — the latter is O(n²) and
	// stalled for tens of seconds on large single-file diffs.
	lines := strings.Split(raw, "\n")
	var result []FileDiff
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		if fd := parseFileSection(lines[start:end]); fd.Path != "" {
			result = append(result, fd)
		}
	}
	for i, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			flush(i)
			start = i
		}
	}
	flush(len(lines))
	return result
}

func parseFileSection(lines []string) FileDiff {
	// Drop trailing empty lines left by splitting a newline-terminated section so
	// they aren't mistaken for hunk-body lines.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
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

// extractPath resolves the file's path from a diff section, preferring the new
// side (+++ b/…) and falling back to the old side (--- a/…) for a deletion or the
// "diff --git" header for a change with no ±±± lines (binary, mode-only). It
// decodes git's C-style quoting so non-ASCII and special-character paths (which
// git renders as "b/caf\303\251.js") aren't dropped.
func extractPath(lines []string) string {
	var newPath, oldPath, headerPath string
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++ "):
			newPath = decodeDiffPath(strings.TrimPrefix(line, "+++ "), "b/")
		case strings.HasPrefix(line, "--- "):
			oldPath = decodeDiffPath(strings.TrimPrefix(line, "--- "), "a/")
		case strings.HasPrefix(line, "diff --git "):
			if headerPath == "" {
				headerPath = gitHeaderPath(line)
			}
		}
	}
	switch {
	case newPath != "": // added or modified
		return newPath
	case oldPath != "": // deleted (+++ was /dev/null)
		return oldPath
	default:
		return headerPath
	}
}

// decodeDiffPath turns a raw "--- "/"+++ " operand into a repo-relative path:
// it C-unquotes a quoted operand, trims git's space-disambiguation trailing tab
// from an unquoted one, maps /dev/null to "", and strips the a/ or b/ prefix.
func decodeDiffPath(s, prefix string) string {
	if strings.HasPrefix(s, `"`) {
		s = unquoteGitPath(s)
	} else {
		s = strings.TrimSuffix(s, "\t")
	}
	if s == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(s, prefix)
}

// gitHeaderPath best-effort extracts the new path from a "diff --git a/… b/…"
// line, used only when no ±±± lines exist. Spaces make the unquoted form
// ambiguous, so this is a fallback; quoted operands are unquoted.
func gitHeaderPath(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	if strings.HasPrefix(rest, `"`) {
		// "a/old" "b/new": the new path is the last space-separated quoted token.
		if i := strings.LastIndex(rest, ` "`); i >= 0 {
			return decodeDiffPath(rest[i+1:], "b/")
		}
		return ""
	}
	if parts := strings.SplitN(rest, " b/", 2); len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// unquoteGitPath decodes a git C-quoted path (core.quotePath, on by default):
// the octal escapes it emits for bytes >0x7f plus \a \b \t \n \v \f \r \" \\.
func unquoteGitPath(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	inner := s[1 : len(s)-1]
	var b []byte
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c != '\\' {
			b = append(b, c)
			continue
		}
		i++
		if i >= len(inner) {
			break
		}
		switch e := inner[i]; e {
		case 'a':
			b = append(b, '\a')
		case 'b':
			b = append(b, '\b')
		case 't':
			b = append(b, '\t')
		case 'n':
			b = append(b, '\n')
		case 'v':
			b = append(b, '\v')
		case 'f':
			b = append(b, '\f')
		case 'r':
			b = append(b, '\r')
		case '"':
			b = append(b, '"')
		case '\\':
			b = append(b, '\\')
		default:
			if isOctal(e) && i+2 < len(inner) && isOctal(inner[i+1]) && isOctal(inner[i+2]) {
				b = append(b, byte(int(e-'0')<<6|int(inner[i+1]-'0')<<3|int(inner[i+2]-'0')))
				i += 2
			} else {
				b = append(b, e)
			}
		}
	}
	return string(b)
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }

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

// RawRangeDiffAll returns the whole-repo unified diff between two commits — no
// pathspec, and --find-renames forced, so rename detection is on even under
// diff.renames=false config (a per-file pathspec or a config-disabled detection
// would render a renamed file as 100% added, disagreeing with the listing).
// Callers parse it once and index by path.
func RawRangeDiffAll(repoRoot, base, target string) (string, error) {
	cmd := exec.Command("git", "diff", "--no-color", "--find-renames", base, target)
	cmd.Dir = repoRoot
	return runGitDiff(cmd, "git diff range")
}

// rawRender is the plain header + body of a hunk, used as the graceful-degradation
// output when structural rendering is unavailable or empty.
func (h Hunk) rawRender() string {
	return h.Header + "\n" + strings.Join(h.Lines, "\n")
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

// RawWorktreeDiff returns the whole working-tree (or staged) unified diff — no
// pathspec, and --find-renames forced (see RawRangeDiffAll), so rename
// detection stays on and one subprocess covers every file. Callers parse it
// once and index by path. Untracked files aren't included (git diff omits
// them); those are handled separately.
func RawWorktreeDiff(repoRoot string, staged bool) (string, error) {
	args := []string{"diff", "--no-color", "--find-renames"}
	if staged {
		args = append(args, "--staged")
	}
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
