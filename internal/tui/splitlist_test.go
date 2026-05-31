package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func testCfg() SplitConfig[string] {
	return SplitConfig[string]{
		Screen: "x", ListTitle: "l", MinList: 20, MaxList: 60,
		Row:          func(s string, w int, sel, c bool) string { return s },
		Match:        func(s string) string { return s },
		PreviewTitle: func(s string) string { return s },
		CacheKey:     func(s string) string { return s },
	}
}

func plainView(m SplitList[string]) string { return ansi.Strip(m.View()) }

// TestPreviewCachingAndStaleness covers the preview lifecycle: stale content is
// cleared on selection change, cached items reload instantly without a spinner,
// and results from superseded requests are discarded.
func TestPreviewCachingAndStaleness(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha", "beta"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 56, Height: 9})

	// Load alpha's preview.
	m, _ = m.Update(PreviewMsg{Content: "DIFF-ALPHA", ReqID: m.reqID})
	if !strings.Contains(plainView(m), "DIFF-ALPHA") {
		t.Fatal("alpha content not shown")
	}

	// Switching to beta must immediately drop alpha's (now stale) diff and enter
	// the loading state until beta's preview arrives.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if strings.Contains(plainView(m), "DIFF-ALPHA") {
		t.Error("stale alpha content still shown after switching to beta")
	}
	if !m.loading {
		t.Error("expected loading=true while beta's preview is pending")
	}
	m, _ = m.Update(PreviewMsg{Content: "DIFF-BETA", ReqID: m.reqID})

	// Returning to alpha is a cache hit: instant content, no loading state.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.loading {
		t.Error("cache hit should not enter loading state")
	}
	if !strings.Contains(plainView(m), "DIFF-ALPHA") {
		t.Error("cached alpha not shown instantly")
	}

	// A PreviewMsg from a superseded request (wrong ReqID) is ignored.
	before := plainView(m)
	m, _ = m.Update(PreviewMsg{Content: "STALE", ReqID: -1})
	if plainView(m) != before {
		t.Error("stale PreviewMsg (wrong ReqID) was applied")
	}
}

// TestClearCacheAndReload verifies the layout-toggle path drops cached previews
// so the current selection reloads.
func TestClearCacheAndReload(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 56, Height: 9})
	m, _ = m.Update(PreviewMsg{Content: "V1", ReqID: m.reqID})

	var cmd tea.Cmd
	m, cmd = m.ClearCacheAndReload()
	if !m.loading || cmd == nil {
		t.Error("ClearCacheAndReload should re-enter loading and request a preview")
	}
}

// TestWidthInCacheKey verifies a preview cached at one pane width is not reused
// at another (difftastic bakes the layout into its output).
func TestWidthInCacheKey(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 56, Height: 9})
	m, _ = m.Update(PreviewMsg{Content: "NARROW", ReqID: m.reqID})

	// Resize: the cached preview is for the old width, so this must reload.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 9})
	if !m.loading {
		t.Error("resize should miss the width-keyed cache and reload")
	}
}
