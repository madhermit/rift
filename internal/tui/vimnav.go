package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// VimNav provides vim-style viewport navigation (gg, G, Ctrl+d/u/f/b, {/}).
// Embed in any TUI model with a scrollable viewport.
type VimNav struct {
	pendingG       bool
	sectionOffsets []int
}

// HandleKey processes vim navigation keys on the viewport.
// Returns true if the key was consumed.
func (v *VimNav) HandleKey(vp *viewport.Model, msg tea.KeyMsg) bool {
	if v.pendingG {
		v.pendingG = false
		if msg.Type == tea.KeyRunes && string(msg.Runes) == "g" {
			vp.GotoTop()
			return true
		}
	}

	switch msg.Type {
	case tea.KeyCtrlD:
		vp.HalfPageDown()
		return true
	case tea.KeyCtrlU:
		vp.HalfPageUp()
		return true
	case tea.KeyCtrlF:
		vp.PageDown()
		return true
	case tea.KeyCtrlB:
		vp.PageUp()
		return true
	case tea.KeyRunes:
		switch string(msg.Runes) {
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
	}
	return false
}

// SetContent updates the viewport content and scans for section offsets.
func (v *VimNav) SetContent(vp *viewport.Model, content string) {
	vp.SetContent(content)
	v.sectionOffsets = scanSectionOffsets(content)
}

func scanSectionOffsets(content string) []int {
	var offsets []int
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "diff --git ") || strings.HasPrefix(line, "───") {
			offsets = append(offsets, i)
		}
	}
	return offsets
}

func jumpToSection(vp *viewport.Model, offsets []int, dir int) {
	if len(offsets) == 0 {
		return
	}
	current := vp.YOffset
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
