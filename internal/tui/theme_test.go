package tui

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// TestApplyTheme verifies the adaptive palette resolves to its dark members by
// default and to its light members after ApplyTheme(false), which is all that a
// terminal's background reply flips.
func TestApplyTheme(t *testing.T) {
	defer ApplyTheme(true) // restore the dark default for the rest of the suite

	ApplyTheme(true)
	if !sameColor(Text, lipgloss.Color("252")) || !sameColor(Bright, lipgloss.Color("15")) {
		t.Error("dark palette: Text/Bright should resolve to their dark members")
	}

	ApplyTheme(false)
	if !sameColor(Text, lipgloss.Color("235")) || !sameColor(Bright, lipgloss.Color("16")) {
		t.Error("light palette: Text/Bright should resolve to their light members")
	}
	if sameColor(Text, lipgloss.Color("252")) {
		t.Error("light palette should not resolve to the dark Text value")
	}
}

// TestThemeOverride covers RIFT_THEME parsing: light/dark force a palette, an
// unset or unknown value defers to detection.
func TestThemeOverride(t *testing.T) {
	tests := []struct {
		env              string
		wantDark, wantOK bool
	}{
		{"light", false, true},
		{"dark", true, true},
		{"", false, false},
		{"bogus", false, false},
	}
	for _, tt := range tests {
		t.Setenv("RIFT_THEME", tt.env)
		dark, ok := themeOverride()
		if dark != tt.wantDark || ok != tt.wantOK {
			t.Errorf("themeOverride(%q) = %v, %v; want %v, %v", tt.env, dark, ok, tt.wantDark, tt.wantOK)
		}
	}
}

func TestPanelDimensions(t *testing.T) {
	const w, h = 30, 8
	out := Panel("title", "", "line one\nline two", w, h, true, Scrollbar{})
	if got := lipgloss.Height(out); got != h {
		t.Errorf("Panel height = %d, want %d", got, h)
	}
	for i, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("Panel row %d width = %d, want %d", i, got, w)
		}
	}
}

func TestPanelEmbeddedTitle(t *testing.T) {
	out := Panel("commits", "", "", 24, 4, true, Scrollbar{})
	top := strings.Split(out, "\n")[0]
	if !strings.Contains(top, "commits") {
		t.Errorf("top border missing title: %q", top)
	}
	// A title too wide for the panel falls back to a plain border.
	plain := Panel("a very long title that will not fit", "", "", 10, 4, false, Scrollbar{})
	if strings.Contains(strings.Split(plain, "\n")[0], "title") {
		t.Errorf("expected oversized title to be dropped, got %q", plain)
	}
}

func TestFooterDimensions(t *testing.T) {
	const w = 50
	out := Footer(w, "1/3", [][2]string{{"q", "quit"}, {"/", "filter"}})
	rows := strings.Split(out, "\n")
	if len(rows) != FooterRows {
		t.Fatalf("Footer rows = %d, want %d", len(rows), FooterRows)
	}
	if got := lipgloss.Width(rows[0]); got != w {
		t.Errorf("Footer rule width = %d, want %d", got, w)
	}
	if !strings.Contains(out, "quit") || !strings.Contains(out, "filter") {
		t.Errorf("Footer missing hints: %q", out)
	}
}

func TestHeaderRows(t *testing.T) {
	out := Header("log", "difftastic", 40)
	if got := lipgloss.Height(out); got != HeaderRows {
		t.Errorf("Header height = %d, want %d", got, HeaderRows)
	}
	if !strings.Contains(out, "rift") || !strings.Contains(out, "log") {
		t.Errorf("Header missing app/screen name: %q", out)
	}
}

func TestMarker(t *testing.T) {
	if lipgloss.Width(Marker(true)) != 1 || lipgloss.Width(Marker(false)) != 1 {
		t.Error("Marker must always occupy exactly one cell")
	}
}
