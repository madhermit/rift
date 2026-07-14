package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// VimNav provides vim-style viewport navigation (gg, G, Ctrl+d/u/f/b, {/}).
// Embed in any TUI model with a scrollable viewport.
type VimNav struct {
	pendingG bool
	sections []section // file boundaries within the displayed content
	lines    []string  // finalized display lines (banners stripped), the render source
}

// Lines returns the finalized display lines — exactly what the viewport
// renders, banner-stripped. The mouse selection reads them to copy content.
func (v *VimNav) Lines() []string { return v.lines }

// section is a file boundary in the preview: the displayed line where the file
// begins, and its label (path) taken from the non-displayed "── path ──" banner.
type section struct {
	offset int
	label  string
}

// HandleKey processes vim navigation keys on the viewport.
// Returns true if the key was consumed.
func (v *VimNav) HandleKey(vp *viewport.Model, msg tea.KeyPressMsg) bool {
	if v.pendingG {
		v.pendingG = false
		if msg.String() == "g" {
			vp.GotoTop()
			return true
		}
	}

	switch msg.String() {
	case "ctrl+d":
		vp.HalfPageDown()
		return true
	case "ctrl+u":
		vp.HalfPageUp()
		return true
	case "ctrl+f":
		vp.PageDown()
		return true
	case "ctrl+b":
		vp.PageUp()
		return true
	case "g":
		v.pendingG = true
		return true
	case "G":
		vp.GotoBottom()
		return true
	case "{":
		jumpToSection(vp, v.sections, -1)
		return true
	case "}":
		jumpToSection(vp, v.sections, 1)
		return true
	}
	return false
}

// HandleListKey processes the vim jump keys (gg, G, ctrl+d/u/f/b) for a windowed
// LIST selection rather than a scrolling viewport: gg/G jump to the first/last
// item, ctrl+d/u by half the visible window, and ctrl+f/b by a full window. It
// returns the new selection index and whether the key was consumed. It shares
// pendingG with HandleKey — only one of the list/preview panes is focused at a
// time, so a pending 'g' can't straddle both.
func (v *VimNav) HandleListKey(msg tea.KeyPressMsg, selected, total, window int) (int, bool) {
	if v.pendingG {
		v.pendingG = false
		if msg.String() == "g" {
			return 0, true // gg → first item
		}
	}
	if window < 1 {
		window = 1
	}
	switch msg.String() {
	case "g":
		v.pendingG = true
		return selected, true
	case "G":
		return clampIndex(total-1, total), true
	case "ctrl+d":
		return clampIndex(selected+window/2, total), true
	case "ctrl+u":
		return clampIndex(selected-window/2, total), true
	case "ctrl+f":
		return clampIndex(selected+window, total), true
	case "ctrl+b":
		return clampIndex(selected-window, total), true
	}
	return selected, false
}

// SetContent replaces the viewport content (a hardwrapped string) and records
// file-section boundaries, returning the displayed (banner-stripped) lines so the
// caller can locate text within them (e.g. to anchor-scroll); the line indices
// line up with viewport offsets. SectionBanner lines mark file boundaries but are
// NOT displayed: the pane's own difftastic/git header is the visible boundary
// while scrolling, and the section's path is surfaced in the panel legend instead
// (see SplitList.currentSection). Each banner is stripped from the shown content
// and recorded as the section starting at the next displayed line.
func (v *VimNav) SetContent(vp *viewport.Model, content string) []string {
	v.sections = nil
	v.lines = nil
	if content != "" {
		v.appendLines(strings.Split(content, "\n"))
	}
	vp.SetContent(strings.Join(v.lines, "\n"))
	return v.lines
}

// AppendContent finalizes an already-hardwrapped block of streamed content into
// the display, extracting any section banners. It does not touch the viewport —
// RenderStreaming redraws once the chunk's trailing line is also known. Used by
// SplitList.appendPreview so each chunk is wrapped once rather than re-wrapping
// the whole buffer.
func (v *VimNav) AppendContent(wrapped string) {
	v.appendLines(strings.Split(wrapped, "\n"))
}

// RenderStreaming redraws the viewport from the finalized display lines plus the
// still-growing (already-wrapped) trailing line, and returns the finalized lines.
// The trailing line is shown but not section-scanned or finalized, so the drawn
// content matches a full re-wrap of the same buffer.
func (v *VimNav) RenderStreaming(vp *viewport.Model, tail string) []string {
	content := tail // nothing finalized yet: the buffer is a single partial line
	if len(v.lines) > 0 {
		// The terminated lines, then the current (maybe empty) trailing one.
		content = strings.Join(v.lines, "\n") + "\n" + tail
	}
	vp.SetContent(content)
	return v.lines
}

// appendLines files each already-wrapped line into the display: a section banner
// records a boundary (and is not shown), anything else becomes a display line.
func (v *VimNav) appendLines(wrapped []string) {
	for _, line := range wrapped {
		if label, ok := bannerLabel(line); ok {
			v.sections = append(v.sections, section{offset: len(v.lines), label: label})
			continue
		}
		v.lines = append(v.lines, line)
	}
}

// bannerLabel returns the file path from a SectionBanner line ("── path ──────").
func bannerLabel(line string) (string, bool) {
	if rest, ok := strings.CutPrefix(ansi.Strip(line), bannerPrefix); ok {
		return strings.TrimRight(rest, "─ "), true
	}
	return "", false
}

// currentIndex returns the index of the section the viewport top row y is in,
// or -1 before the first section (e.g. scrolling a commit header).
func (v *VimNav) currentIndex(y int) int {
	cur := -1
	for i, s := range v.sections {
		if s.offset > y {
			break
		}
		cur = i
	}
	return cur
}

// CurrentSection returns the label of the file the viewport is currently in, for
// the panel legend. Empty before the first section or when there are none.
func (v *VimNav) CurrentSection(y int) string {
	if cur := v.currentIndex(y); cur >= 0 {
		return v.sections[cur].label
	}
	return ""
}

// SectionProgress reports the current file's 1-based position as "N/M" (file of
// total files) for the panel legend, or "" before the first file / when there
// are no sections.
func (v *VimNav) SectionProgress(y int) string {
	if cur := v.currentIndex(y); cur >= 0 {
		return fmt.Sprintf("%d/%d", cur+1, len(v.sections))
	}
	return ""
}

// ScrollbarFor builds a Scrollbar describing a viewport's scroll state, for
// rendering a thumb in its panel border.
func ScrollbarFor(vp *viewport.Model) Scrollbar {
	return Scrollbar{Total: vp.TotalLineCount(), Visible: vp.Height(), Offset: vp.YOffset()}
}

func jumpToSection(vp *viewport.Model, sections []section, dir int) {
	if len(sections) == 0 {
		return
	}
	current := vp.YOffset()
	if dir > 0 {
		for _, s := range sections {
			if s.offset > current {
				vp.SetYOffset(s.offset)
				return
			}
		}
	} else {
		for i := len(sections) - 1; i >= 0; i-- {
			if sections[i].offset < current {
				vp.SetYOffset(sections[i].offset)
				return
			}
		}
	}
}
