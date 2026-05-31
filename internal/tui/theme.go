package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Palette — a blue accent over a neutral gray scale, shared by every screen so
// the chrome stays consistent. Kept on ANSI 256 indices for broad terminal
// support.
var (
	Accent = lipgloss.Color("39")  // primary highlight (blue)
	Text   = lipgloss.Color("252") // primary foreground
	Subtle = lipgloss.Color("245") // secondary foreground
	Faint  = lipgloss.Color("238") // borders, rules, dim chrome
	Bright = lipgloss.Color("15")  // selected foreground

	Green  = lipgloss.Color("2") // added / staged
	Red    = lipgloss.Color("1") // deleted / unstaged
	Yellow = lipgloss.Color("3") // modified
)

var (
	panelBorder = lipgloss.RoundedBorder()

	borderActive = lipgloss.NewStyle().Foreground(Accent)
	borderDim    = lipgloss.NewStyle().Foreground(Faint)
	titleActive  = lipgloss.NewStyle().Foreground(Accent).Bold(true)
	titleDim     = lipgloss.NewStyle().Foreground(Subtle)

	appNameStyle = lipgloss.NewStyle().Foreground(Accent).Bold(true)
	dotStyle     = lipgloss.NewStyle().Foreground(Faint)
	screenStyle  = lipgloss.NewStyle().Foreground(Text).Bold(true)
	contextStyle = lipgloss.NewStyle().Foreground(Subtle)

	ruleStyle   = lipgloss.NewStyle().Foreground(Faint)
	keyStyle    = lipgloss.NewStyle().Foreground(Accent)
	keyDescDim  = lipgloss.NewStyle().Foreground(Subtle)
	statusStyle = lipgloss.NewStyle().Foreground(Text)
	markerStyle = lipgloss.NewStyle().Foreground(Accent)

	SelectedTextStyle = lipgloss.NewStyle().Foreground(Bright)
	NormalTextStyle   = lipgloss.NewStyle().Foreground(Subtle)
)

// StatusStyle returns the foreground style for a git status word
// ("Added"/"Deleted"/"Modified"/"Renamed"). Unknown statuses are dim.
func StatusStyle(status string) lipgloss.Style {
	switch status {
	case "Added":
		return lipgloss.NewStyle().Foreground(Green)
	case "Deleted":
		return lipgloss.NewStyle().Foreground(Red)
	case "Modified":
		return lipgloss.NewStyle().Foreground(Yellow)
	case "Renamed":
		return lipgloss.NewStyle().Foreground(Accent)
	default:
		return lipgloss.NewStyle().Foreground(Subtle)
	}
}

// Marker returns the leading glyph for a list row: an accent bar for the
// selected row, a blank column otherwise. It always occupies one cell.
func Marker(selected bool) string {
	if selected {
		return markerStyle.Render("▎")
	}
	return " "
}

// Header renders the two-row top bar: "rift · <screen>" on the left and an
// optional dim context (e.g. the diff engine, current branch) on the right.
func Header(screen, context string, width int) string {
	left := " " + appNameStyle.Render("rift")
	if screen != "" {
		left += dotStyle.Render(" · ") + screenStyle.Render(screen)
	}
	right := contextStyle.Render(context)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right + " "
	return line + "\n" // second (blank) row as breathing room
}

// Footer renders the two-row bottom bar: a faint rule then a status segment
// followed by styled "key desc" hints separated by dim dots.
func Footer(width int, status string, hints [][2]string) string {
	rule := ruleStyle.Render("╶" + strings.Repeat("─", maxInt(0, width-2)) + "╴")

	var parts []string
	if status != "" {
		parts = append(parts, statusStyle.Render(status))
	}
	var hintStrs []string
	for _, h := range hints {
		hintStrs = append(hintStrs, keyStyle.Render(h[0])+" "+keyDescDim.Render(h[1]))
	}
	if len(hintStrs) > 0 {
		parts = append(parts, strings.Join(hintStrs, ruleStyle.Render(" · ")))
	}

	line := " " + strings.Join(parts, "   ")
	line = ansi.Truncate(line, width, "")
	return rule + "\n" + line
}

// FooterContent renders the bottom rule above an arbitrary content line, used
// for the filter-input prompt while filtering.
func FooterContent(width int, content string) string {
	rule := ruleStyle.Render("╶" + strings.Repeat("─", maxInt(0, width-2)) + "╴")
	return rule + "\n " + ansi.Truncate(content, maxInt(0, width-1), "")
}

// Panel renders body inside a rounded box of the given OUTER width and height,
// with an optional title embedded in the top border. When active, the border
// and title use the accent color; otherwise they are dimmed. Body lines are
// clipped and padded to the inner width; extra rows are blank-filled.
func Panel(title, body string, width, height int, active bool) string {
	if width < 2 {
		width = 2
	}
	if height < 2 {
		height = 2
	}
	inner := width - 2
	innerH := height - 2

	bs, ts := borderDim, titleDim
	if active {
		bs, ts = borderActive, titleActive
	}

	// Top border with optional embedded title: ╭─ title ──────╮
	var top string
	if title != "" && inner >= lipgloss.Width(title)+4 {
		title = ts.Render(title)
		fill := inner - 2 - lipgloss.Width(title) // 2 = leading "─" + space, trailing handled below
		top = bs.Render(panelBorder.TopLeft+panelBorder.Top) +
			" " + title + " " +
			bs.Render(strings.Repeat(panelBorder.Top, maxInt(0, fill-1))+panelBorder.TopRight)
	} else {
		top = bs.Render(panelBorder.TopLeft + strings.Repeat(panelBorder.Top, inner) + panelBorder.TopRight)
	}

	left := bs.Render(panelBorder.Left)
	right := bs.Render(panelBorder.Right)
	lines := strings.Split(body, "\n")
	rows := make([]string, 0, innerH+2)
	rows = append(rows, top)
	for r := 0; r < innerH; r++ {
		content := ""
		if r < len(lines) {
			content = lines[r]
		}
		content = ansi.Truncate(content, inner, "")
		if pad := inner - lipgloss.Width(content); pad > 0 {
			content += strings.Repeat(" ", pad)
		}
		rows = append(rows, left+content+right)
	}
	rows = append(rows, bs.Render(panelBorder.BottomLeft+strings.Repeat(panelBorder.Bottom, inner)+panelBorder.BottomRight))
	return strings.Join(rows, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
