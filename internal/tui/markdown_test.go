package tui

import (
	"strings"
	"testing"
)

func TestMarkdownPlain(t *testing.T) {
	// color=false strips inline markers and joins wrapped paragraphs.
	got := Markdown("Use `SplitList` and **read** the docs.", 80, false)
	want := "  Use SplitList and read the docs."
	if got != want {
		t.Errorf("Markdown() = %q, want %q", got, want)
	}
}

func TestMarkdownBullets(t *testing.T) {
	got := Markdown("Intro\n\n- one\n- two", 80, false)
	want := "  Intro\n\n  • one\n  • two"
	if got != want {
		t.Errorf("Markdown() =\n%q\nwant\n%q", got, want)
	}
}

func TestMarkdownParagraphRejoin(t *testing.T) {
	// Consecutive lines join into one paragraph and wrap to width.
	got := Markdown("alpha\nbeta\ngamma", 80, false)
	if got != "  alpha beta gamma" {
		t.Errorf("Markdown() = %q", got)
	}
	// A blank line separates paragraphs.
	got = Markdown("alpha\n\nbeta", 80, false)
	if got != "  alpha\n\n  beta" {
		t.Errorf("Markdown() = %q", got)
	}
}

func TestMarkdownWraps(t *testing.T) {
	got := Markdown("one two three four five", 12, false)
	// width 12 → wrap at 10; every rendered line stays within width.
	for _, line := range strings.Split(got, "\n") {
		if len(line) > 12 {
			t.Errorf("line exceeds width: %q (%d)", line, len(line))
		}
	}
}
