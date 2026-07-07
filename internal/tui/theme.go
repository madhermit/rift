package tui

import (
	"image/color"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	addStyle = lipgloss.NewStyle().Foreground(Green)
	delStyle = lipgloss.NewStyle().Foreground(Red)
)

// DiffStat renders "+A -B" with green/red counts, or "" when both are zero.
func DiffStat(added, deleted int) string {
	if added == 0 && deleted == 0 {
		return ""
	}
	var parts []string
	if added > 0 {
		parts = append(parts, addStyle.Render("+"+strconv.Itoa(added)))
	}
	if deleted > 0 {
		parts = append(parts, delStyle.Render("-"+strconv.Itoa(deleted)))
	}
	return strings.Join(parts, " ")
}

// ColorEnabled reports whether ANSI color output is enabled (i.e. NO_COLOR is
// unset). Callers pass the result to the diff engine and styling helpers.
func ColorEnabled() bool { return os.Getenv("NO_COLOR") == "" }

// TextStyle returns the list-row text style for a selected vs unselected row.
func TextStyle(selected bool) lipgloss.Style {
	if selected {
		return SelectedTextStyle
	}
	return NormalTextStyle
}

// RenderPath renders a file path with a dimmed directory and an emphasized
// basename — so the eye lands on the filename — truncated to fit width.
func RenderPath(path string, width int, selected bool) string {
	path = TruncatePath(path, width)
	dir, base := path, ""
	if i := strings.LastIndex(path, "/"); i >= 0 {
		dir, base = path[:i+1], path[i+1:]
	} else {
		dir, base = "", path
	}
	dirStyle, baseStyle := pathDirStyle, pathBaseStyle
	if selected {
		dirStyle, baseStyle = pathDirSelStyle, pathBaseSelStyle
	}
	return dirStyle.Render(dir) + baseStyle.Render(base)
}

// lightBackground selects the light palette when true; the zero value (dark) is
// the default, so rift stays dark until detection or RIFT_THEME says otherwise.
// It is written only by ApplyTheme — from the single-threaded Update loop or at
// program start — and read (atomically) while rendering, so the one write can't
// race the many render-time reads.
var lightBackground atomic.Bool

// adaptiveColor resolves to its light or dark member at render time (lipgloss
// calls RGBA when it draws), so one ApplyTheme flip re-themes every style that
// captured it — no per-style rebuild, and it works across packages since the
// exported palette colors below are the shared instances every screen uses.
type adaptiveColor struct{ dark, light color.Color }

func (c adaptiveColor) RGBA() (r, g, b, a uint32) {
	if lightBackground.Load() {
		return c.light.RGBA()
	}
	return c.dark.RGBA()
}

// adaptive pairs a dark and a light ANSI-256 value into a background-aware color.
func adaptive(dark, light string) color.Color {
	return adaptiveColor{dark: lipgloss.Color(dark), light: lipgloss.Color(light)}
}

// ApplyTheme selects the light or dark palette. Called once per program from
// ThemeInit (a RIFT_THEME override) or from themeFilter on the terminal's
// BackgroundColorMsg — both on the Update goroutine. Only the palette selector
// flips; the styles that captured the palette colors are untouched.
func ApplyTheme(dark bool) { lightBackground.Store(!dark) }

// Palette — a blue accent over a neutral gray scale, shared by every screen so
// the chrome stays consistent. The gray ramp and the accent flip between light
// and dark terminals (see adaptiveColor); the semantic hues stay on ANSI base
// indices so the terminal's own light/dark theme keeps them legible.
var (
	Accent = adaptive("39", "26")   // primary highlight (blue)
	Text   = adaptive("252", "235") // primary foreground
	Subtle = adaptive("245", "240") // secondary foreground
	Faint  = adaptive("238", "250") // borders, rules, dim chrome
	Bright = adaptive("15", "16")   // selected foreground

	Green   = lipgloss.Color("2") // added / staged
	Red     = lipgloss.Color("1") // deleted / unstaged
	Yellow  = lipgloss.Color("3") // modified
	Magenta = lipgloss.Color("5") // unstaged hunk sidebar
)

// bannerPrefix leads every SectionBanner; VimNav.bannerLabel keys section
// detection on it, so the producer and consumer share this one literal.
const bannerPrefix = "── "

// SectionBanner renders a full-width divider embedding a file path — a section
// marker that the preview strips from the body (VimNav.SetContent) and pins in
// the panel legend. The label is truncated so the banner always stays on one
// line (a wrapped banner would break section detection).
func SectionBanner(label string, width int) string {
	if avail := width - 4; avail >= 1 { // prefix + trailing space
		label = TruncatePath(label, avail)
	}
	head := bannerRuleStyle.Render(bannerPrefix) + bannerLabelStyle.Render(label) + " "
	if pad := width - lipgloss.Width(head); pad > 0 {
		head += bannerRuleStyle.Render(strings.Repeat("─", pad))
	}
	return head
}

var (
	bannerRuleStyle  = lipgloss.NewStyle().Foreground(Faint)
	bannerLabelStyle = lipgloss.NewStyle().Foreground(Accent).Bold(true)

	// Scrollbar thumb in a panel's right border. The border doubles as the track,
	// so the thumb must contrast with it: Bright against the focused Accent
	// border, Subtle against the dim Faint border.
	scrollThumbDim    = lipgloss.NewStyle().Foreground(Subtle)
	scrollThumbActive = lipgloss.NewStyle().Foreground(Bright)

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

	// Path rendering: dim directory, emphasized basename (see RenderPath).
	pathDirStyle     = lipgloss.NewStyle().Foreground(Faint)
	pathBaseStyle    = lipgloss.NewStyle().Foreground(Text)
	pathDirSelStyle  = lipgloss.NewStyle().Foreground(Subtle)
	pathBaseSelStyle = lipgloss.NewStyle().Foreground(Bright).Bold(true)
)

var (
	toggleOn  = lipgloss.NewStyle().Foreground(Accent)
	toggleOff = lipgloss.NewStyle().Foreground(Subtle)
)

// ToggleTitle renders a two-option toggle for a panel title (e.g.
// "unstaged"/"staged"): the active option in accent, the other dimmed, so the
// title names the current mode and shows the alternative the toggle key (`s`)
// switches to.
func ToggleTitle(left, right string, rightActive bool) string {
	ls, rs := toggleOn, toggleOff
	if rightActive {
		ls, rs = toggleOff, toggleOn
	}
	return ls.Render(left) + toggleOff.Render("/") + rs.Render(right)
}

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

// Marker returns the leading glyph for a list row: a bold accent bar for the
// selected row, a blank column otherwise. It always occupies one cell. The
// half-block (▌) is intentionally distinct from the heavy state bar (┃) used in
// the stage hunk gutter, so a selection never reads as a staged/unstaged mark.
func Marker(selected bool) string {
	if selected {
		return markerStyle.Render("▌")
	}
	return " "
}

// breadcrumbSep joins screen segments in a header breadcrumb (rift ❯ diff ❯ tests).
const breadcrumbSep = " ❯ "

// Breadcrumb joins screen segments for a SplitConfig.Screen so the header renders
// them as a "a ❯ b" trail (e.g. a lens over a parent screen).
func Breadcrumb(segments ...string) string {
	return strings.Join(segments, breadcrumbSep)
}

// Header renders the two-row top bar: a "rift ❯ <screen>" chevron breadcrumb on
// the left and an optional dim context (e.g. branch · engine) on the right, then
// a thin rule separating it from the content (mirroring the footer).
func Header(screen, context string, width int) string {
	left := " " + appNameStyle.Render("rift")
	if screen != "" {
		for _, seg := range strings.Split(screen, breadcrumbSep) {
			left += dotStyle.Render(breadcrumbSep) + screenStyle.Render(seg)
		}
	}
	right := contextStyle.Render(context)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right + " " + "\n" + ruleLine(width)
}

// ruleLine is the thin horizontal separator drawn under the header and above the
// footer hints.
func ruleLine(width int) string {
	return ruleStyle.Render("╶" + strings.Repeat("─", maxInt(0, width-2)) + "╴")
}

// HeaderContext joins the current branch with a screen-specific context (e.g.
// the diff engine) for the header's right side, omitting either when empty.
func HeaderContext(branch, rest string) string {
	switch {
	case branch == "":
		return rest
	case rest == "":
		return branch
	default:
		return branch + " · " + rest
	}
}

// ContextLabel is the header right-context for a diff-backed screen: the engine
// name, prefixed with the target ref when viewing a committed range (target is
// "" for a working-tree/staged diff).
func ContextLabel(target, engineName string) string {
	if target == "" {
		return engineName
	}
	return target + " · " + engineName
}

// Footer renders the two-row bottom bar: a faint rule then a status segment
// followed by styled "key desc" hints separated by dim dots. When the hints
// don't fit, the trailing hints (e.g. "? help", "q quit") are kept and the
// middle is dropped with an ellipsis, so help and quit stay discoverable.
func Footer(width int, status string, hints [][2]string) string {
	rule := ruleLine(width)
	sep := ruleStyle.Render(" · ")
	render := func(h [2]string) string { return keyStyle.Render(h[0]) + " " + keyDescDim.Render(h[1]) }

	left := " "
	if status != "" {
		left += statusStyle.Render(status) + "   "
	}

	full := left + joinHints(hints, sep, render)
	if len(hints) == 0 || lipgloss.Width(full) <= width {
		return rule + "\n" + ansi.Truncate(full, width, "")
	}

	// Overflow: pin the trailing hints (help/quit), fill leading ones, drop the
	// middle with an ellipsis.
	tailN := minInt(2, len(hints))
	tail := joinHints(hints[len(hints)-tailN:], sep, render)
	ellipsis := ruleStyle.Render(" … ")
	budget := width - lipgloss.Width(left) - lipgloss.Width(ellipsis) - lipgloss.Width(tail)

	var kept []string
	used := 0
	for _, h := range hints[:len(hints)-tailN] {
		seg := render(h)
		cost := lipgloss.Width(seg)
		if len(kept) > 0 {
			cost += lipgloss.Width(sep)
		}
		if used+cost > budget {
			break
		}
		used += cost
		kept = append(kept, seg)
	}

	line := left + strings.Join(kept, sep) + ellipsis + tail
	return rule + "\n" + ansi.Truncate(line, width, "")
}

func joinHints(hints [][2]string, sep string, render func([2]string) string) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = render(h)
	}
	return strings.Join(parts, sep)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FooterContent renders the bottom rule above an arbitrary content line, used
// for the filter-input prompt while filtering.
func FooterContent(width int, content string) string {
	rule := ruleLine(width)
	return rule + "\n " + ansi.Truncate(content, maxInt(0, width-1), "")
}

// PreviewHelpKeys and BasicHelpKeys are the always-available keys listed at the
// bottom of the help overlay. Screens with a scrollable preview use the former.
var (
	// PreviewHelpKeys is the navigation reference shown (in full) in every preview
	// screen's help overlay, including alternate keys.
	PreviewHelpKeys = [][2]string{
		{"j/k  ↑↓", "move / scroll"},
		{"J/K  ⇧↑↓  ]/[", "next / prev item"},
		{"gg/G", "top / bottom"},
		{"{/}", "prev / next section"},
		{"ctrl+d/u", "scroll half-page"},
		{"ctrl+f/b", "scroll page"},
		{"esc", "back"},
	}
	BasicHelpKeys = [][2]string{{"esc", "back"}}
)

// HelpView renders a keybinding overlay: header + a panel listing the screen's
// hints (shown in full, unlike the footer) plus the extra global keys, with a
// "press any key to close" footer.
func HelpView(screen, context string, hints, extra [][2]string, width, contentHeight int) string {
	header := Header(screen, context, width)
	panel := Panel("keybindings", "", helpBody(hints, extra), width, contentHeight, true, Scrollbar{})
	footer := FooterContent(width, keyDescDim.Render("press any key to close"))
	return lipgloss.JoinVertical(lipgloss.Left, header, panel, footer)
}

func helpBody(hints, extra [][2]string) string {
	rows := append([][2]string{}, hints...)
	rows = append(rows, [2]string{"", ""})
	rows = append(rows, extra...)

	keyW := 0
	for _, h := range rows {
		if w := lipgloss.Width(h[0]); w > keyW {
			keyW = w
		}
	}
	var b strings.Builder
	for _, h := range rows {
		if h[0] == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("  " + keyStyle.Render(h[0]) + strings.Repeat(" ", keyW-lipgloss.Width(h[0])) + "   " + keyDescDim.Render(h[1]) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Panel renders body inside a rounded box of the given OUTER width and height,
// with an optional title embedded in the top border. When active, the border
// and title use the accent color; otherwise they are dimmed. Body lines are
// clipped and padded to the inner width; extra rows are blank-filled.
// Scrollbar describes a scrollable region's state for rendering a thumb in a
// panel's right border. The zero value (or Total <= Visible) renders no thumb.
type Scrollbar struct {
	Total   int // total content lines
	Visible int // visible rows
	Offset  int // index of the first visible line
}

// thumb returns the [start,end) inner-row indices the scrollbar thumb covers,
// or ok=false when the content fits and no thumb should render.
func (s Scrollbar) thumb(rows int) (start, end int, ok bool) {
	if rows <= 0 || s.Visible <= 0 || s.Total <= s.Visible {
		return 0, 0, false
	}
	// Total > Visible here, so size is always < rows.
	size := rows * s.Visible / s.Total
	if size < 1 {
		size = 1
	}
	maxOff := s.Total - s.Visible
	pos := 0
	if maxOff > 0 {
		pos = s.Offset * (rows - size) / maxOff
	}
	if pos > rows-size {
		pos = rows - size // Offset past the end (e.g. content shrank)
	}
	return pos, pos + size, true
}

// ListWindow computes the scroll offset (first visible row) for a windowed list
// that keeps the selected item on screen, along with the matching Scrollbar for
// the panel border. Shared by the list panes (splitlist, stage).
func ListWindow(selected, total, visible int) (offset int, bar Scrollbar) {
	if selected >= visible {
		offset = selected - visible + 1
	}
	return offset, Scrollbar{Total: total, Visible: visible, Offset: offset}
}

func Panel(title, rightTitle, body string, width, height int, active bool, sb Scrollbar) string {
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
	thumbStart, thumbEnd, hasThumb := sb.thumb(innerH)

	// Top border with an optional left title and right annotation:
	// ╭─ title ──────── right ─╮. When they don't fit, fall back to a plain border.
	leftSeg, rightSeg := "", ""
	if title != "" {
		leftSeg = " " + ts.Render(title) + " "
	}
	if rightTitle != "" {
		rightSeg = " " + ts.Render(rightTitle) + " "
	}
	midFill := inner - 2 - lipgloss.Width(leftSeg) - lipgloss.Width(rightSeg)
	if midFill < 0 { // drop the secondary right annotation first, the title only if still too wide
		rightSeg = ""
		midFill = inner - 2 - lipgloss.Width(leftSeg)
	}
	if midFill < 0 {
		leftSeg = ""
		midFill = inner - 2
	}
	top := bs.Render(panelBorder.TopLeft+panelBorder.Top) + leftSeg +
		bs.Render(strings.Repeat(panelBorder.Top, maxInt(0, midFill))) + rightSeg +
		bs.Render(panelBorder.Top+panelBorder.TopRight)

	left := bs.Render(panelBorder.Left)
	right := bs.Render(panelBorder.Right)
	thumb := right
	if hasThumb {
		thumbStyle := scrollThumbDim
		if active {
			thumbStyle = scrollThumbActive
		}
		thumb = thumbStyle.Render("▐")
	}
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
		rb := right
		if hasThumb && r >= thumbStart && r < thumbEnd {
			rb = thumb
		}
		rows = append(rows, left+content+rb)
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
