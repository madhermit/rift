package tui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	mdCodeStyle = lipgloss.NewStyle().Foreground(Accent)
	mdBoldStyle = lipgloss.NewStyle().Bold(true)

	mdInlineCode = regexp.MustCompile("`([^`]+)`")
	mdBold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdBullet     = regexp.MustCompile(`^\s*[-*+]\s+(.*)`)
)

// Markdown renders a small, dependency-free subset of markdown to ANSI:
// blank-line-separated paragraphs (word-wrapped), bullet lists, inline `code`,
// and **bold**. Output is indented two columns. This covers virtually all real
// commit-message bodies without pulling in a full CommonMark stack.
func Markdown(text string, width int, color bool) string {
	if width <= 0 {
		width = 80
	}
	const indent = "  "
	wrap := width - len(indent)
	if wrap < 8 {
		wrap = width
	}

	var out []string
	var para []string
	flush := func() {
		if len(para) == 0 {
			return
		}
		joined := mdInline(strings.Join(para, " "), color)
		out = append(out, indentLines(ansi.Wordwrap(joined, wrap, ""), indent))
		para = nil
	}

	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.TrimSpace(line) == "":
			flush()
			out = append(out, "")
		case mdBullet.MatchString(line):
			flush()
			item := mdInline(strings.TrimSpace(mdBullet.FindStringSubmatch(line)[1]), color)
			wrapped := strings.Split(ansi.Wordwrap(item, wrap-2, ""), "\n")
			out = append(out, indent+"• "+wrapped[0])
			for _, extra := range wrapped[1:] {
				out = append(out, indent+"  "+extra)
			}
		default:
			para = append(para, strings.TrimSpace(line))
		}
	}
	flush()
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

func indentLines(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}

// mdInline applies (or, when color is off, strips) inline code and bold markers.
func mdInline(s string, color bool) string {
	if !color {
		s = mdInlineCode.ReplaceAllString(s, "$1")
		return mdBold.ReplaceAllString(s, "$1")
	}
	s = mdInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		return mdCodeStyle.Render(strings.Trim(m, "`"))
	})
	return mdBold.ReplaceAllStringFunc(s, func(m string) string {
		return mdBoldStyle.Render(strings.Trim(m, "*"))
	})
}
