package logui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madhermit/rift/internal/diff"
	"github.com/madhermit/rift/internal/git"
)

type stubEngine struct{}

func (stubEngine) Name() string { return "difftastic" }
func (stubEngine) Diff(context.Context, string, string, diff.DiffOpts) (string, error) {
	return "", nil
}
func (stubEngine) DiffHunks(context.Context, []diff.Hunk, string, string, bool, int) []string {
	return nil
}

// TestViewFillsWindow guards the header+content+footer math: the composed view
// must be exactly as tall as the terminal, at a range of sizes.
func TestViewFillsWindow(t *testing.T) {
	commits := []git.CommitInfo{
		{Hash: "a1b2c3d", Date: "2026-05-30 12:01", Message: "Fix fd leak"},
		{Hash: "d4e5f6a", Date: "2026-05-30 09:12", Message: "Bump Charm ecosystem"},
	}
	for _, size := range []struct{ w, h int }{{80, 24}, {120, 40}, {60, 16}} {
		m := New(nil, stubEngine{}, commits)
		next, _ := m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
		content := next.(Model).View().Content
		if got := lipgloss.Height(content); got != size.h {
			t.Errorf("at %dx%d: view height = %d, want %d", size.w, size.h, got, size.h)
		}
	}
}
