package tui

import tea "charm.land/bubbletea/v2"

// PreviewChild is a SplitList-backed model embedded in a parent that hosts and
// routes to it — the diff lens (files↔tests) and the log drilldown. The parent
// uses Capturing/ShowingHelp to know when not to run its own key handlers,
// InMode to know when not to take esc for itself, and CancelPreview to stop the
// child's stream before hiding it.
type PreviewChild interface {
	Update(tea.Msg) (tea.Model, tea.Cmd)
	View() tea.View
	Capturing() bool
	ShowingHelp() bool
	InMode() bool
	CancelPreview() tea.Model
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
