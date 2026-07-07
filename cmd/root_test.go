package cmd

import "testing"

// TestMenuLoopExits covers the menu-loop decision: it re-enters the menu after a
// screen exits cleanly, and stops when the user quit the menu or a git write ran.
func TestMenuLoopExits(t *testing.T) {
	tests := []struct {
		name      string
		selected  string
		actionRan bool
		want      bool
	}{
		{"quit menu", "", false, true},
		{"screen exited cleanly", "diff", false, false},
		{"git write ran", "stash", true, true},
		{"browsed and quit", "log", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := menuLoopExits(tt.selected, tt.actionRan); got != tt.want {
				t.Errorf("menuLoopExits(%q, %v) = %v, want %v", tt.selected, tt.actionRan, got, tt.want)
			}
		})
	}
}
