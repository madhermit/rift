package tui

import tea "charm.land/bubbletea/v2"

// PreviewChild is a SplitList-backed model hosted by the files↔tests lens. The parent
// uses Capturing/ShowingHelp to know when not to run its own key handlers,
// InMode to know when not to take esc for itself, and CancelPreview to stop the
// child's stream before hiding it (ResumePreview picks it back up on return, as
// the messages it was waiting on are dropped while hidden). Settings and ApplySettings carry the engine
// and layout across a switch between children.
type PreviewChild interface {
	Update(tea.Msg) (tea.Model, tea.Cmd)
	View() tea.View
	Capturing() bool
	ShowingHelp() bool
	InMode() bool
	CancelPreview() tea.Model
	ResumePreview() (tea.Model, tea.Cmd)
	Settings() ViewSettings
	ApplySettings(ViewSettings) (tea.Model, tea.Cmd)
}

// WrapPreviewCmd wraps the async preview messages a child's command emits (and
// recurses into batches) with wrap, so a parent hosting a SplitList-backed child
// can tag and guard that child's traffic — its request IDs start at 0 and would
// otherwise collide with the parent's or a sibling child's. Control messages
// (quit, editor exec) and spinner ticks (scoped by their own ID) pass through
// untouched so the runtime still sees them.
func WrapPreviewCmd(cmd tea.Cmd, wrap func(tea.Msg) tea.Msg) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			tagged := make(tea.BatchMsg, len(msg))
			for i, c := range msg {
				tagged[i] = WrapPreviewCmd(c, wrap)
			}
			return tagged
		case SelectionChangedMsg, PreviewMsg, StreamReadyMsg, ChunkMsg:
			return wrap(msg)
		default:
			return msg
		}
	}
}
