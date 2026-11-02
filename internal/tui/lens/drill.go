package lensui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/madhermit/rift/internal/tui"
)

// Drill hosts a lens as a per-item view under a list screen (a commit in the log,
// an entry in the stash), so those screens offer the same files↔tests view as
// `rift diff` instead of each wiring up their own. The zero value is closed.
type Drill struct {
	lens Model
	open bool
}

// OpenDrill opens lens at the given size, on its tests side when tests is set.
func OpenDrill(lens Model, width, height int, tests bool) (Drill, tea.Cmd) {
	d := Drill{lens: lens, open: true}
	d, sized := d.update(tea.WindowSizeMsg{Width: width, Height: height})
	if !tests {
		return d, sized
	}
	nm, shown := d.lens.ShowTests()
	d.lens = nm.(Model)
	return d, tea.Batch(sized, shown)
}

// Open reports whether the drill is showing.
func (d Drill) Open() bool { return d.open }

// Owns reports whether msg should go to the drill rather than its host. While
// open the host keeps only a resize (which it forwards) and its own list's preview
// traffic, which is bare: the lens tags everything it starts, so the two can't be
// confused.
func (d Drill) Owns(msg tea.Msg) bool {
	if !d.open {
		return false
	}
	switch msg.(type) {
	case tea.WindowSizeMsg, tui.SelectionChangedMsg, tui.PreviewMsg, tui.StreamReadyMsg, tui.ChunkMsg:
		return false
	}
	return true
}

// Update feeds the drill a message it owns. closed reports that esc just took it
// back to the host, which should then call BackToList.
func (d Drill) Update(msg tea.Msg) (next Drill, cmd tea.Cmd, closed bool) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "esc" && !d.lens.InMode() {
		d.lens = d.lens.CancelPreview().(Model)
		d.open = false
		return d, nil, true
	}
	d, cmd = d.update(msg)
	return d, cmd, false
}

func (d Drill) update(msg tea.Msg) (Drill, tea.Cmd) {
	nm, cmd := d.lens.Update(msg)
	d.lens = nm.(Model)
	return d, cmd
}

// Resize forwards a window size to an open drill; the host handles its own list.
func (d Drill) Resize(msg tea.WindowSizeMsg) (Drill, tea.Cmd) {
	if !d.open {
		return d, nil
	}
	return d.update(msg)
}

// Settings is the engine and layout the drill last rendered with.
func (d Drill) Settings() tui.ViewSettings { return d.lens.Settings() }

// BackToList settles a host's list after its drill closed, returning the settings
// to keep: whatever was chosen while drilled carries over. The list's preview was
// frozen (and possibly cut off mid-stream) meanwhile, so it reloads either way — a
// cache hit if it had finished and nothing changed.
func BackToList[T any](d Drill, list tui.SplitList[T], current tui.ViewSettings) (tui.SplitList[T], tui.ViewSettings, tea.Cmd) {
	settings := d.Settings()
	var cmd tea.Cmd
	if settings.Same(current) {
		list, cmd = list.Reload()
		return list, current, cmd
	}
	list, cmd = list.SetContext(settings.Engines.Name()).ClearCacheAndReload()
	return list, settings, cmd
}

// View renders the drill.
func (d Drill) View() tea.View { return d.lens.View() }
