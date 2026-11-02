package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func typeKeys(m SplitList[string], s string) SplitList[string] {
	for _, r := range s {
		m, _ = m.Update(key(r))
	}
	return m
}

// searchable builds a laid-out list whose preview is 60 numbered lines with a
// few plantable needles, focused on the preview pane and scrolled to the top.
func searchable(t *testing.T, content string) SplitList[string] {
	t.Helper()
	m := NewSplitList(testCfg(), []string{"alpha", "beta"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m, _ = m.Update(PreviewMsg{Content: content, ReqID: m.reqID})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !m.Reading() {
		t.Fatal("tab should focus the preview")
	}
	return m
}

func numberedLines(n int, plant map[int]string) string {
	var b strings.Builder
	for i := range n {
		if s, ok := plant[i]; ok {
			b.WriteString(s + "\n")
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// `/` is pane-sensitive: it narrows the list from the list pane and searches the
// rendered diff from the preview. Same gesture, scoped to where you are.
func TestSearchSlashIsPaneSensitive(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha", "beta", "gamma"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})

	m, _ = m.Update(key('/'))
	if !m.filtering || m.searching {
		t.Fatalf("/ over the list should filter (filtering=%v searching=%v)", m.filtering, m.searching)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(key('/'))
	if m.filtering || !m.searching {
		t.Fatalf("/ over the preview should search (filtering=%v searching=%v)", m.filtering, m.searching)
	}
	if !m.Capturing() {
		t.Error("a live search should count as capturing the keyboard")
	}
}

func TestSearchFindsAndSteps(t *testing.T) {
	// Tall enough that every match can reach the top row: the viewport clamps its
	// offset near the end of the content, which would hide the step under a jump
	// that couldn't happen.
	m := searchable(t, numberedLines(120, map[int]string{5: "needle one", 30: "needle two", 50: "needle three"}))

	m, _ = m.Update(key('/'))
	m = typeKeys(m, "needle")
	if want := []int{5, 30, 50}; fmt.Sprint(m.matches) != fmt.Sprint(want) {
		t.Fatalf("matches = %v, want %v", m.matches, want)
	}
	if m.viewport.YOffset() != 5 {
		t.Errorf("first match should be at the top row, offset = %d", m.viewport.YOffset())
	}
	if got := m.SearchStatus(); got != "match 1/3" {
		t.Errorf("searchStatus = %q, want %q", got, "match 1/3")
	}

	// Accept, then walk with n/N — wrapping at both ends.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.searching || m.query != "needle" {
		t.Fatalf("enter should close the box but keep the query (searching=%v query=%q)", m.searching, m.query)
	}
	for _, step := range []struct {
		k      rune
		wantAt int
		want   int
	}{{'n', 2, 30}, {'n', 3, 50}, {'n', 1, 5}, {'N', 3, 50}} {
		m, _ = m.Update(key(step.k))
		if m.matchAt+1 != step.wantAt {
			t.Errorf("%c: match %d, want %d", step.k, m.matchAt+1, step.wantAt)
		}
		if m.viewport.YOffset() != step.want {
			t.Errorf("%c: offset = %d, want %d", step.k, m.viewport.YOffset(), step.want)
		}
	}
}

func TestSearchSmartCase(t *testing.T) {
	content := numberedLines(20, map[int]string{3: "Needle upper", 9: "needle lower"})
	tests := []struct {
		query string
		want  []int
	}{
		{"needle", []int{3, 9}}, // all-lowercase: case-insensitive
		{"Needle", []int{3}},    // an uppercase letter makes it case-sensitive
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			m := searchable(t, content)
			m, _ = m.Update(key('/'))
			m = typeKeys(m, tt.query)
			if fmt.Sprint(m.matches) != fmt.Sprint(tt.want) {
				t.Errorf("matches for %q = %v, want %v", tt.query, m.matches, tt.want)
			}
		})
	}
}

func TestSearchNoMatch(t *testing.T) {
	m := searchable(t, numberedLines(30, nil))
	m, _ = m.Update(key('/'))
	m = typeKeys(m, "zzz")
	if len(m.matches) != 0 {
		t.Fatalf("matches = %v, want none", m.matches)
	}
	if got := m.SearchStatus(); got != "no match" {
		t.Errorf("searchStatus = %q, want %q", got, "no match")
	}
	if m.viewport.YOffset() != 0 {
		t.Errorf("a miss should leave the reader where they were, offset = %d", m.viewport.YOffset())
	}
}

// esc means two different things by design: cancelling mid-type undoes the jumps
// incremental search made, while clearing an accepted search leaves the reader
// wherever they navigated to.
func TestSearchEscape(t *testing.T) {
	content := numberedLines(60, map[int]string{40: "needle"})

	m := searchable(t, content)
	m.viewport.SetYOffset(10)
	m, _ = m.Update(key('/'))
	m = typeKeys(m, "needle")
	if m.viewport.YOffset() != 40 {
		t.Fatalf("search should have jumped to the match, offset = %d", m.viewport.YOffset())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.viewport.YOffset() != 10 {
		t.Errorf("esc while typing should return to the origin, offset = %d", m.viewport.YOffset())
	}
	if m.query != "" || m.searching {
		t.Error("esc should end the search")
	}

	m = searchable(t, content)
	m, _ = m.Update(key('/'))
	m = typeKeys(m, "needle")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.viewport.YOffset() != 40 {
		t.Errorf("esc after accepting should stay put, offset = %d", m.viewport.YOffset())
	}
	if m.query != "" {
		t.Errorf("esc should clear the query, got %q", m.query)
	}
}

// A search names lines in the preview it was run against, so changing selection
// must drop it rather than leave it pointing into content that is gone.
func TestSearchResetsOnSelectionChange(t *testing.T) {
	m := searchable(t, numberedLines(30, map[int]string{7: "needle"}))
	m, _ = m.Update(key('/'))
	m = typeKeys(m, "needle")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	m, _ = m.Update(key('J')) // step to the next item
	if m.query != "" || len(m.matches) != 0 || m.matchAt != -1 {
		t.Errorf("a new selection should reset the search (query=%q matches=%v at=%d)", m.query, m.matches, m.matchAt)
	}
}

// A query typed against a half-streamed preview keeps finding hits as the rest
// arrives: each chunk extends the match list rather than triggering a rescan.
func TestSearchExtendsWhileStreaming(t *testing.T) {
	m := NewSplitList(testCfg(), []string{"alpha"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m, _ = m.Update(PreviewMsg{Content: "needle A\nfiller\n", ReqID: m.reqID, Append: true})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	m, _ = m.Update(key('/'))
	m = typeKeys(m, "needle")
	if fmt.Sprint(m.matches) != "[0]" {
		t.Fatalf("matches so far = %v, want [0]", m.matches)
	}
	m, _ = m.Update(PreviewMsg{Content: "filler\nneedle B\n", ReqID: m.reqID, Append: true})
	if want := "[0 3]"; fmt.Sprint(m.matches) != want {
		t.Errorf("matches after the next chunk = %v, want %s", m.matches, want)
	}
	if got := m.SearchStatus(); got != "match 1/2" {
		t.Errorf("searchStatus = %q, want %q", got, "match 1/2")
	}
}

// The legend's right slot is contested: a live search outranks the status a
// parent tracks (the diff screen's hunk position), and yields it back on clear.
func TestSearchStatusOutranksParentStatus(t *testing.T) {
	m := searchable(t, numberedLines(30, map[int]string{4: "needle"}))
	m = m.SetPreviewStatus("hunk 2/5")
	if got := m.previewRightText(); got != "hunk 2/5" {
		t.Fatalf("previewRightText = %q, want the parent status", got)
	}
	m, _ = m.Update(key('/'))
	m = typeKeys(m, "needle")
	if got := m.previewRightText(); got != "match 1/1" {
		t.Errorf("previewRightText during search = %q, want the match count", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := m.previewRightText(); got != "hunk 2/5" {
		t.Errorf("previewRightText after clearing = %q, want the parent status back", got)
	}
}

// While a query is being typed, screen keys must reach the input, not the view.
func TestSearchCapturesKeys(t *testing.T) {
	m := searchable(t, numberedLines(40, map[int]string{6: "gg needle"}))
	m, _ = m.Update(key('/'))
	m = typeKeys(m, "gg")
	if m.query != "gg" {
		t.Errorf("query = %q, want %q — g was consumed as a nav key", m.query, "gg")
	}
	if m.viewport.YOffset() != 6 {
		t.Errorf("offset = %d: gg scrolled instead of searching", m.viewport.YOffset())
	}
}

// esc has two audiences: the input that owns the keyboard, and a parent that
// reads esc as "back" (the log drilldown). An accepted search has closed its
// input but still holds the highlight and n/N, so esc must still land here —
// while screen keys stay live, which is why the two states are asked separately.
func TestSearchInModeOutlastsCapturing(t *testing.T) {
	m := searchable(t, numberedLines(30, map[int]string{4: "needle"}))
	if m.Capturing() || m.InMode() {
		t.Fatal("a plain preview is in no mode")
	}

	m, _ = m.Update(key('/'))
	m = typeKeys(m, "needle")
	if !m.Capturing() || !m.InMode() {
		t.Error("while typing, the search both captures keys and owns esc")
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Capturing() {
		t.Error("an accepted search must not keep capturing keys")
	}
	if !m.InMode() {
		t.Error("an accepted search still owns esc — its highlight and n/N are live")
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.InMode() {
		t.Error("esc should retire the search")
	}
}
