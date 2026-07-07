package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

// NewProgram builds a bubbletea program wired for adaptive light/dark theming: a
// message filter applies the palette when the terminal answers the background
// color query. Every TUI entrypoint runs through this, so the wiring lives in one
// place rather than being copy-pasted into five models.
func NewProgram(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
	opts = append(opts, tea.WithFilter(themeFilter))
	return tea.NewProgram(m, opts...)
}

// ThemeInit is the command each top-level model returns from Init to select its
// palette: with RIFT_THEME forced it applies immediately and queries nothing;
// otherwise it asks the terminal for its background color, answered by a
// BackgroundColorMsg that themeFilter turns into an ApplyTheme call. A terminal
// that never replies (or a non-TTY) leaves the default dark palette in place.
func ThemeInit() tea.Cmd {
	if dark, ok := themeOverride(); ok {
		ApplyTheme(dark)
		return nil
	}
	return tea.RequestBackgroundColor
}

// themeFilter applies the detected palette on the terminal's background-color
// reply. It runs inside the program's single-threaded Update loop (via
// tea.WithFilter), so mutating the palette selector here is safe, and it passes
// every message through untouched.
func themeFilter(_ tea.Model, msg tea.Msg) tea.Msg {
	if bg, ok := msg.(tea.BackgroundColorMsg); ok {
		if _, forced := themeOverride(); !forced {
			ApplyTheme(bg.IsDark())
		}
	}
	return msg
}

// themeOverride reports the palette forced by RIFT_THEME (light|dark), and
// whether it was set at all.
func themeOverride() (dark, ok bool) {
	switch os.Getenv("RIFT_THEME") {
	case "light":
		return false, true
	case "dark":
		return true, true
	default:
		return false, false
	}
}
