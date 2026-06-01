package tui

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"shorter than max", "abc", 10, "abc"},
		{"equal to max", "abcde", 5, "abcde"},
		{"trailing ellipsis", "abcdefgh", 6, "abc..."},
		{"max at ellipsis boundary", "abcdef", 3, "abc"},
		{"max below ellipsis", "abcdef", 2, "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Truncate(tt.s, tt.max); got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestTruncatePath(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"shorter than max", "a/b", 10, "a/b"},
		{"equal to max", "a/b/c", 5, "a/b/c"},
		{"shorten leading dirs keeps filename", "internal/tui/model.go", 12, "i/t/model.go"},
		{"shorten only as needed", "internal/tui/diff/model.go", 18, "i/t/diff/model.go"},
		{"long filename elided behind ellipsis", "abcdefgh", 4, "…fgh"},
		{"no dirs to shorten", "averylongname.go", 6, "…me.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncatePath(tt.s, tt.max); got != tt.want {
				t.Errorf("TruncatePath(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}
