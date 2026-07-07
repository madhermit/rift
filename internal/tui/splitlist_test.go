package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func testCfg() SplitConfig[string] {
	return SplitConfig[string]{
		Screen: "x", ListTitle: "l", NavFraction: 30,
		Row:          func(s string, w int, sel bool) string { return s },
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
	m, _ = m.Update(tea.WindowSizeMsg{Width: 56, Height: 24})

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

func chunkBytes(s string, n int) []string {
	if n < 1 {
		n = 1
	}
	size := (len(s) + n - 1) / n
	var chunks []string
	for i := 0; i < len(s); i += size {
		end := min(i+size, len(s))
		chunks = append(chunks, s[i:end])
	}
	return chunks
}

// TestStreamedAppendMatchesFullRewrap guards the incremental streamed-append path
// (which hardwraps only each chunk) against a full re-wrap of the whole buffer.
// The content has section banners, long lines that wrap at the pane width, and
// wide runes, and is fed in many small chunks so a wrapped line straddles chunk
// boundaries — exactly the case a naive per-chunk wrap gets wrong.
func TestStreamedAppendMatchesFullRewrap(t *testing.T) {
	banner := func(p string) string { return SectionBanner(p, 38) + "\n" }
	long := strings.Repeat("abcd ", 20) // wraps several times at width 38
	wide := strings.Repeat("你好世界", 6)   // wide runes, wraps
	raw := banner("pkg/file/one.go") + long + "\n" +
		banner("pkg/file/two.go") + wide + "\n" +
		"a short trailing line\n"

	// Streamed: feed the content in many awkward chunks via Append messages.
	ms := NewSplitList(testCfg(), []string{"item"})
	ms, _ = ms.Update(tea.WindowSizeMsg{Width: 40, Height: 60})
	for _, c := range chunkBytes(raw, 13) {
		ms, _ = ms.Update(PreviewMsg{Content: c, ReqID: ms.reqID, Append: true})
	}

	// Full: one non-append message carrying the whole content.
	mf := NewSplitList(testCfg(), []string{"item"})
	mf, _ = mf.Update(tea.WindowSizeMsg{Width: 40, Height: 60})
	mf, _ = mf.Update(PreviewMsg{Content: raw, ReqID: mf.reqID})

	if got, want := plainView(ms), plainView(mf); got != want {
		t.Errorf("streamed append != full rewrap\n--- streamed ---\n%s\n--- full ---\n%s", got, want)
	}
	// The banners must be extracted as sections in both, not shown as body lines.
	if len(ms.vim.sections) != 2 {
		t.Errorf("streamed: got %d sections, want 2", len(ms.vim.sections))
	}
	if got := ms.currentSection(); got != "pkg/file/one.go" {
		t.Errorf("streamed: currentSection at top = %q, want pkg/file/one.go", got)
	}
}

func lineIndex(v, sub string) int {
	for i, line := range strings.Split(v, "\n") {
		if strings.Contains(line, sub) {
			return i
		}
	}
	return -1
}

// TestStripLayout covers the strip layout: it lands in surveying with the
// list on top and the preview below; tab focuses the preview full-screen (strip
// hidden, position in the title); and J steps items in place.
func TestStripLayout(t *testing.T) {
	cfg := testCfg()
	m := NewSplitList(cfg, []string{"alpha", "beta"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(PreviewMsg{Content: "DIFF-ALPHA", ReqID: m.reqID})

	// Surveying: both list items sit above the preview.
	v := plainView(m)
	if m.active != splitListPane {
		t.Fatal("should land in surveying (list focused)")
	}
	la, lb, pv := lineIndex(v, "alpha"), lineIndex(v, "beta"), lineIndex(v, "DIFF-ALPHA")
	if la < 0 || lb < 0 || pv < 0 || la >= pv || lb >= pv {
		t.Errorf("surveying: list items should sit above the preview (alpha=%d beta=%d preview=%d)", la, lb, pv)
	}

	// tab focuses the preview full-screen: strip hidden, position in the title.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if v = plainView(m); m.active != splitPreviewPane || !strings.Contains(v, "1/2") {
		t.Error("tab: should focus the preview with the position in its title")
	}
	if lineIndex(v, "beta") >= 0 {
		t.Error("reading: the non-selected item should be hidden")
	}

	// J steps to the next item; the new preview shows once loaded.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	m, _ = m.Update(PreviewMsg{Content: "DIFF-BETA", ReqID: m.reqID})
	if v = plainView(m); !strings.Contains(v, "DIFF-BETA") || !strings.Contains(v, "2/2") {
		t.Error("J: should step to the next item in the preview")
	}
}

// TestStripFilter covers inline filtering of the strip: / narrows the
// list and esc restores it.
func TestStripFilter(t *testing.T) {
	cfg := testCfg()
	m := NewSplitList(cfg, []string{"alpha", "beta", "gamma"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.Filtering() {
		t.Fatal("/ should start filtering")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if len(m.filtered) != 1 || m.filtered[0] != "gamma" {
		t.Fatalf("filter 'g' should narrow to gamma, got %v", m.filtered)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Filtering() || len(m.filtered) != 3 {
		t.Errorf("esc should clear the filter (filtering=%v, %d items)", m.Filtering(), len(m.filtered))
	}
}

// TestPreviewAppend verifies streamed chunks grow the preview (keeping earlier
// content) and that a chunk from a superseded request is ignored.
func TestPreviewAppend(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 56, Height: 24})

	m, _ = m.Update(PreviewMsg{Content: "FILE-A\n", ReqID: m.reqID, Append: true})
	if !strings.Contains(plainView(m), "FILE-A") {
		t.Fatal("first streamed chunk not shown")
	}
	m, _ = m.Update(PreviewMsg{Content: "FILE-B\n", ReqID: m.reqID, Append: true})
	if v := plainView(m); !strings.Contains(v, "FILE-A") || !strings.Contains(v, "FILE-B") {
		t.Error("append should grow the preview, keeping earlier content")
	}
	before := plainView(m)
	m, _ = m.Update(PreviewMsg{Content: "STALE", ReqID: -1, Append: true})
	if plainView(m) != before {
		t.Error("stale append (wrong ReqID) was applied")
	}
}

// TestClearCacheAndReload verifies the layout-toggle path drops cached previews
// so the current selection reloads.
func TestClearCacheAndReload(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 56, Height: 24})
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
	m, _ = m.Update(tea.WindowSizeMsg{Width: 56, Height: 24})
	m, _ = m.Update(PreviewMsg{Content: "NARROW", ReqID: m.reqID})

	// Resize: the cached preview is for the old width, so this must reload.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 9})
	if !m.loading {
		t.Error("resize should miss the width-keyed cache and reload")
	}
}

// TestFilterKeepsSelection verifies re-filtering keeps the cursor on the same
// item when it survives the new filter, and falls back to the top otherwise.
func TestFilterKeepsSelection(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha", "alpaca", "beta"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) // select alpaca
	if m.SelectedKey() != "alpaca" {
		t.Fatalf("setup: selected %q, want alpaca", m.SelectedKey())
	}

	// "alp" keeps both alpha and alpaca; the selection must stay on alpaca.
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "alp" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if m.SelectedKey() != "alpaca" {
		t.Errorf("filter should keep selection on alpaca, got %q", m.SelectedKey())
	}

	// Narrowing to "alpha" drops alpaca, so the selection resets to the survivor.
	for _, r := range "ha" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if m.SelectedKey() != "alpha" {
		t.Errorf("when the selection doesn't survive, reset to the first item; got %q", m.SelectedKey())
	}
}

// TestListPaneVimJumps covers gg/G and ctrl+d moving the LIST selection while the
// list pane is focused (as opposed to scrolling the preview).
func TestListPaneVimJumps(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"a", "b", "c", "d", "e"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if m.selected != 4 {
		t.Errorf("G should select the last item, got %d", m.selected)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if m.selected != 0 {
		t.Errorf("gg should select the first item, got %d", m.selected)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.selected == 0 {
		t.Error("ctrl+d should move the selection down by half a window")
	}
}

// TestNewGutter covers the preview anchor's parse of difftastic's new-side line
// gutter in both layouts: inline (new number left-indented past the margin) and
// side-by-side (new number in a right gutter). The anchor must read the NEW
// side, never the old one, so an insertion that shifts the two apart still lands
// the scroll on the right row. Right-aligned numbers of differing widths share
// an end column, which is why detection keys on the end, not the start.
func TestNewGutter(t *testing.T) {
	inline := []string{
		"167    ",                       // old-side context, margin gutter
		"168   test \"excludes hold\"",  // old-side, removed
		"   168   test \"covers hold\"", // new-side, end col 6
		"   169     loan = create_loan", // new-side
		"      x := 9999",               // content number, not a gutter
	}
	if got := newGutterEnd(inline); got != 6 {
		t.Fatalf("inline gutter end = %d, want 6", got)
	}
	if got := newLineNum(inline[2], 6); got != 168 {
		t.Errorf("inline new line = %d, want 168", got)
	}
	if got := newLineNum(inline[1], 6); got != -1 {
		t.Errorf("old-side row should have no new line, got %d", got)
	}

	// Side-by-side: old number at the margin, new number in a right gutter that
	// difftastic right-aligns to a fixed end column. Build rows with a constant
	// left field so the new gutter lands consistently; a removed row has "..."
	// (no new side) and one new number is 2-digit to exercise right-alignment.
	rows := []struct{ oldNum, oldTxt, newNum, newTxt string }{
		{"165", "assert_equal 900.0, x", "165", "assert_equal 900.0, x"},
		{"168", `test "excludes hold"`, "168", `test "covers hold"`},
		{"175", "loan.update!(:dont_repay)", "", ""}, // removed: no new side
		{"177", "end", "99", "end"},                  // right-aligned 2-digit
	}
	var sbs []string
	for _, r := range rows {
		right := "..."
		if r.newNum != "" {
			right = fmt.Sprintf("%3s  %s", r.newNum, r.newTxt) // 3-wide right-aligned gutter
		}
		sbs = append(sbs, fmt.Sprintf("%-3s  %-40s%s", r.oldNum, r.oldTxt, right))
	}
	end := newGutterEnd(sbs)
	if got := newLineNum(sbs[1], end); got != 168 {
		t.Errorf("sbs new line = %d, want 168", got)
	}
	if got := newLineNum(sbs[2], end); got != -1 {
		t.Errorf("sbs removed row should have no new line, got %d", got)
	}
	if got := newLineNum(sbs[3], end); got != 99 {
		t.Errorf("sbs right-aligned 2-digit new line = %d, want 99", got)
	}
}
