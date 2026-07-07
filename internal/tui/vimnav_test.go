package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func TestBannerLabel(t *testing.T) {
	tests := []struct {
		line string
		want string
		ok   bool
	}{
		{"── internal/main.go ──────────", "internal/main.go", true},
		{"\x1b[2m── a/b.go ──\x1b[m", "a/b.go", true},
		{"diff --git a/f.go b/f.go", "", false},
		{"─────────────────────", "", false}, // bare rule line, not a banner
		{"just content", "", false},
	}
	for _, tt := range tests {
		got, ok := bannerLabel(tt.line)
		if got != tt.want || ok != tt.ok {
			t.Errorf("bannerLabel(%q) = %q, %v; want %q, %v", tt.line, got, ok, tt.want, tt.ok)
		}
	}
}

func TestVimNav_CurrentSection(t *testing.T) {
	// First section at offset 3 to exercise the "before the first file" case.
	v := VimNav{sections: []section{{offset: 3, label: "a.go"}, {offset: 10, label: "b.go"}}}
	tests := []struct {
		y    int
		want string
	}{
		{0, ""},      // before the first file (e.g. a commit header)
		{3, "a.go"},  // at a.go's start — shown immediately, no scroll needed
		{7, "a.go"},  // within a.go
		{10, "b.go"}, // at b.go's start
		{20, "b.go"}, // within b.go
	}
	for _, tt := range tests {
		if got := v.CurrentSection(tt.y); got != tt.want {
			t.Errorf("CurrentSection(%d) = %q, want %q", tt.y, got, tt.want)
		}
	}
}

func TestVimNav_HandleKey(t *testing.T) {
	// Build content with 100 lines so there's room to scroll
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")

	newViewport := func() viewport.Model {
		vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
		vp.SetContent(content)
		return vp
	}

	keyMsg := func(s string) tea.KeyPressMsg {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}

	t.Run("G goes to bottom", func(t *testing.T) {
		vp := newViewport()
		var v VimNav
		if !v.HandleKey(&vp, keyMsg("G")) {
			t.Fatal("expected handled")
		}
		if vp.YOffset() == 0 {
			t.Error("expected YOffset > 0 after G")
		}
	})

	t.Run("gg goes to top", func(t *testing.T) {
		vp := newViewport()
		vp.SetYOffset(50)
		var v VimNav
		v.HandleKey(&vp, keyMsg("g"))
		if !v.HandleKey(&vp, keyMsg("g")) {
			t.Fatal("expected handled on second g")
		}
		if vp.YOffset() != 0 {
			t.Errorf("expected YOffset=0 after gg, got %d", vp.YOffset())
		}
	})

	t.Run("Ctrl+D half page down", func(t *testing.T) {
		vp := newViewport()
		var v VimNav
		if !v.HandleKey(&vp, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}) {
			t.Fatal("expected handled")
		}
		if vp.YOffset() == 0 {
			t.Error("expected YOffset > 0 after Ctrl+D")
		}
	})

	t.Run("unhandled key returns false", func(t *testing.T) {
		vp := newViewport()
		var v VimNav
		if v.HandleKey(&vp, keyMsg("x")) {
			t.Error("expected not handled for 'x'")
		}
	})
}

func TestVimNav_HandleListKey(t *testing.T) {
	key := func(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: rune(s[0]), Text: s} }
	var v VimNav

	// G jumps to the last index.
	if got, ok := v.HandleListKey(key("G"), 0, 5, 3); !ok || got != 4 {
		t.Errorf("G = %d,%v; want 4,true", got, ok)
	}
	// gg: the first g arms, the second jumps to 0.
	if got, ok := v.HandleListKey(key("g"), 4, 5, 3); !ok || got != 4 {
		t.Errorf("first g = %d,%v; want 4,true (armed, no move)", got, ok)
	}
	if got, ok := v.HandleListKey(key("g"), 4, 5, 3); !ok || got != 0 {
		t.Errorf("second g = %d,%v; want 0,true", got, ok)
	}
	// ctrl+d steps down by half the window.
	if got, ok := v.HandleListKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, 0, 10, 6); !ok || got != 3 {
		t.Errorf("ctrl+d = %d,%v; want 3,true", got, ok)
	}
	// ctrl+f steps down by a full window, ctrl+b up by a full window (clamped).
	if got, ok := v.HandleListKey(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl}, 0, 20, 6); !ok || got != 6 {
		t.Errorf("ctrl+f = %d,%v; want 6,true", got, ok)
	}
	if got, ok := v.HandleListKey(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}, 10, 20, 6); !ok || got != 4 {
		t.Errorf("ctrl+b = %d,%v; want 4,true", got, ok)
	}
	// An unrelated key is not consumed.
	if _, ok := v.HandleListKey(key("x"), 0, 5, 3); ok {
		t.Error("x should not be handled")
	}
}

func TestVimNav_SetContent(t *testing.T) {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	var v VimNav
	content := "line1\n── a.go ──────────\nadiff\n── b.go ──────────\nbdiff\n"
	v.SetContent(&vp, content)

	// Banners are stripped from the displayed content and recorded as sections at
	// the displayed line where each file begins.
	if strings.Contains(vp.View(), "──") {
		t.Errorf("banner not stripped from displayed content:\n%s", vp.View())
	}
	want := []section{{offset: 1, label: "a.go"}, {offset: 2, label: "b.go"}}
	if !slices.Equal(v.sections, want) {
		t.Errorf("sections = %v, want %v", v.sections, want)
	}
}
