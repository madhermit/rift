package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// VimNav provides vim-style viewport navigation (gg, G, Ctrl+d/u/f/b, {/}).
// Embed in any TUI model with a scrollable viewport.
type VimNav struct {
	pendingG       bool
	lines          []string // the current content, split once for offsets/sticky
	sectionOffsets []int
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
		jumpToSection(vp, v.sectionOffsets, -1)
		return true
	case "}":
		jumpToSection(vp, v.sectionOffsets, 1)
		return true
	}
	return false
}

// SetContent updates the viewport content and scans for section offsets,
// splitting the content into lines once for both.
func (v *VimNav) SetContent(vp *viewport.Model, content string) {
	vp.SetContent(content)
	v.lines = strings.Split(content, "\n")
	v.sectionOffsets = scanSectionOffsets(v.lines)
}

// Lines returns the current content split into lines (shared with the sticky
// header so the preview isn't re-split).
func (v *VimNav) Lines() []string { return v.lines }

// SectionOffsets returns the line indices of section boundaries (file banners /
// diff headers) in the current content, for `{`/`}` jumps and sticky headers.
func (v *VimNav) SectionOffsets() []int { return v.sectionOffsets }

// ScrollbarFor builds a Scrollbar describing a viewport's scroll state, for
// rendering a thumb in its panel border.
func ScrollbarFor(vp *viewport.Model) Scrollbar {
	return Scrollbar{Total: vp.TotalLineCount(), Visible: vp.Height(), Offset: vp.YOffset()}
}

func scanSectionOffsets(lines []string) []int {
	var offsets []int
	for i, line := range lines {
		s := ansi.Strip(line)
		// "── " (with the trailing space) is a file banner; a bare run of dashes
		// (a rule line, e.g. the commit header separator) is not a section.
		if strings.Contains(s, "diff --git ") || strings.HasPrefix(s, "── ") {
			offsets = append(offsets, i)
		}
	}
	return offsets
}

func jumpToSection(vp *viewport.Model, offsets []int, dir int) {
	if len(offsets) == 0 {
		return
	}
	current := vp.YOffset()
	if dir > 0 {
		for _, off := range offsets {
			if off > current {
				vp.SetYOffset(off)
				return
			}
		}
	} else {
		for i := len(offsets) - 1; i >= 0; i-- {
			if offsets[i] < current {
				vp.SetYOffset(offsets[i])
				return
			}
		}
	}
}
