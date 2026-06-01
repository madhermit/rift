package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func TestScanSectionOffsets(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []int
	}{
		{"empty", "", nil},
		{"no sections", "hello\nworld\n", nil},
		{
			"git-diff headers",
			"diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\ndiff --git a/util.go b/util.go\n",
			[]int{0, 6},
		},
		{
			"file banner is a section",
			"preamble\n── internal/main.go ──────────\nbody\n",
			[]int{1},
		},
		{
			"bare rule line is not a section",
			"commit abc1234\nAuthor: Alice\n\n─────────────────────\n\nsome diff\n",
			nil,
		},
		{
			"colored git-diff header",
			"some preamble\n\x1b[1mdiff --git a/f.go b/f.go\x1b[m\nindex abc..def\n",
			[]int{1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scanSectionOffsets(strings.Split(tt.content, "\n"))
			if !slices.Equal(got, tt.want) {
				t.Errorf("scanSectionOffsets() = %v, want %v", got, tt.want)
			}
		})
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

func TestVimNav_SetContent(t *testing.T) {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	var v VimNav
	content := "line1\ndiff --git a/f.go b/f.go\nline3\n── b.go ────────────\nline5\n"
	v.SetContent(&vp, content)

	want := []int{1, 3}
	if !slices.Equal(v.sectionOffsets, want) {
		t.Errorf("sectionOffsets = %v, want %v", v.sectionOffsets, want)
	}
}
