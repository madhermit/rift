package tui

// Confirm is an inline yes/no gate for a destructive action. The zero value is
// inactive. A parent stores one, activates it with Ask when a destructive key is
// pressed, shows Prompt in the footer, and routes the next key through Answer:
// "y" confirms, anything else cancels. Either way the gate clears.
type Confirm struct {
	prompt string
	active bool
}

// Ask returns an active confirmation for action (e.g. "drop stash@{0}"); Prompt
// renders it as "drop stash@{0}? (y/n)".
func Ask(action string) Confirm {
	return Confirm{prompt: action, active: true}
}

// Active reports whether a confirmation is pending.
func (c Confirm) Active() bool { return c.active }

// Prompt is the footer text for a pending confirmation, or "" when inactive.
func (c Confirm) Prompt() string {
	if !c.active {
		return ""
	}
	return c.prompt + "? (y/n)"
}

// Answer resolves a pending confirmation for the pressed key: confirmed is true
// only for "y". The returned Confirm is always inactive — any key clears the gate.
func (c Confirm) Answer(key string) (confirmed bool, cleared Confirm) {
	return key == "y", Confirm{}
}
