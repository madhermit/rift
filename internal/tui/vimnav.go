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
}

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

// SetContent updates the viewport content and records file-section boundaries.
// SectionBanner lines mark file boundaries but are NOT displayed: the pane's own
// difftastic/git header is the visible boundary while scrolling, and the
// section's path is surfaced in the panel legend instead (see
// SplitList.currentSection). Each banner is stripped from the shown content and
// recorded as the section starting at the next displayed line.
func (v *VimNav) SetContent(vp *viewport.Model, content string) {
	raw := strings.Split(content, "\n")
	display := make([]string, 0, len(raw))
	v.sections = nil
	for _, line := range raw {
		if label, ok := bannerLabel(line); ok {
			v.sections = append(v.sections, section{offset: len(display), label: label})
			continue
		}
		display = append(display, line)
	}
	vp.SetContent(strings.Join(display, "\n"))
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
