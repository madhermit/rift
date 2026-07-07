package tui

import "testing"

func TestResolveEditor(t *testing.T) {
	tests := []struct {
		name           string
		visual, editor string
		want           string
	}{
		{"both unset falls back to vi", "", "", "vi"},
		{"visual preferred over editor", "code --wait", "vim", "code --wait"},
		{"editor used when visual unset", "", "nvim", "nvim"},
		{"whitespace-only visual is unset", "   ", "nano", "nano"},
		{"whitespace-only both falls back to vi", " ", "\t", "vi"},
		{"visual with surrounding spaces still preferred", "  emacs  ", "vi", "  emacs  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveEditor(tt.visual, tt.editor); got != tt.want {
				t.Errorf("resolveEditor(%q, %q) = %q, want %q", tt.visual, tt.editor, got, tt.want)
			}
		})
	}
}
