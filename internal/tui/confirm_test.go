package tui

import "testing"

func TestConfirm(t *testing.T) {
	var zero Confirm
	if zero.Active() || zero.Prompt() != "" {
		t.Errorf("zero Confirm should be inactive with no prompt, got active=%v prompt=%q", zero.Active(), zero.Prompt())
	}

	c := Ask("drop stash@{0}")
	if !c.Active() {
		t.Fatal("Ask should return an active confirmation")
	}
	if got, want := c.Prompt(), "drop stash@{0}? (y/n)"; got != want {
		t.Errorf("Prompt() = %q, want %q", got, want)
	}

	tests := []struct {
		key           string
		wantConfirmed bool
	}{
		{"y", true},
		{"n", false},
		{"Y", false}, // only lowercase y confirms; anything else cancels
		{"q", false},
		{"esc", false},
	}
	for _, tt := range tests {
		confirmed, cleared := Ask("cherry-pick a1b2c3d").Answer(tt.key)
		if confirmed != tt.wantConfirmed {
			t.Errorf("Answer(%q) confirmed = %v, want %v", tt.key, confirmed, tt.wantConfirmed)
		}
		if cleared.Active() {
			t.Errorf("Answer(%q) should clear the gate", tt.key)
		}
	}
}
