package diff

import (
	"context"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// autoEngine picks an engine per file: difftastic, unless its output says it had
// nothing structural to offer, in which case git's diff reads better. It decides from
// what difftastic printed, not from a guess at the file type, so anything it can't
// classify stays on difftastic.
type autoEngine struct {
	structural Engine
	plain      Engine
}

func (a *autoEngine) Name() string { return "auto" }

func (a *autoEngine) Diff(ctx context.Context, repoRoot string, file File, opts DiffOpts) (string, error) {
	out, err := a.structural.Diff(ctx, repoRoot, file, opts)
	if err != nil {
		return "", err
	}
	reason := plainReason(out)
	if reason == "" {
		return out, nil
	}
	// If git can't show it either (a scope it reads differently, a transient
	// failure), difftastic's answer is still an answer.
	plain, err := a.plain.Diff(ctx, repoRoot, file, opts)
	if err != nil || strings.TrimSpace(plain) == "" {
		return out, nil
	}
	return engineNote(reason, opts.Color) + "\n" + plain, nil
}

// DiffHunks makes the same choice per hunk. Hunks carry no note: stage's plain
// hunks already read as what they are, and one per hunk would be noise.
func (a *autoEngine) DiffHunks(ctx context.Context, hunks []Hunk, filename, baseContent string, color bool, width int) []string {
	results := a.structural.DiffHunks(ctx, hunks, filename, baseContent, color, width)
	plain := a.plain.DiffHunks(ctx, hunks, filename, baseContent, color, width)
	for i, out := range results {
		if plainReason(out) != "" {
			results[i] = plain[i]
		}
	}
	return results
}

// plainReason reads difftastic's output for why git's diff would serve better, or ""
// if it wouldn't. Every way difftastic gives up on structure relabels the file as
// "--- Text": no parser, a binary file, or a limit, which it names in parentheses.
// Its other non-answer is a change that is only formatting, which it reports in place
// of a diff, leaving nothing on screen to review.
func plainReason(out string) string {
	rows := strings.SplitN(ansi.Strip(out), "\n", 4)
	i := strings.LastIndex(rows[0], " --- ") // the last: a path can contain one too
	if i < 0 {
		return ""
	}
	language := rows[0][i+len(" --- "):]
	switch {
	case strings.Contains(language, "PARSE_ERROR_LIMIT"):
		return "difftastic couldn't parse this file"
	case strings.Contains(language, "BYTE_LIMIT"):
		return "too large for difftastic"
	case strings.Contains(language, "GRAPH_LIMIT"):
		return "too complex for difftastic"
	case language == "Text":
		return "difftastic has no parser for this file"
	}
	// A permissions note can sit between the header and the message.
	for _, row := range rows[1:min(len(rows), 3)] {
		if strings.TrimSpace(row) == "No syntactic changes." {
			return "difftastic found no syntactic changes"
		}
	}
	return ""
}

// engineNote is the row above a file auto gave to git, saying why it looks different
// from its neighbors.
func engineNote(reason string, color bool) string {
	note := "git-diff: " + reason
	if color {
		return "\x1b[2m" + note + "\x1b[0m"
	}
	return note
}

// Alternates are the engines `e` cycles to from this one.
func Alternates(e Engine) []Engine {
	switch e := e.(type) {
	case *autoEngine:
		return []Engine{e.structural, e.plain}
	case *fallbackEngine:
		return nil
	}
	return []Engine{NewPlainEngine()}
}
