package tui

import "github.com/madhermit/rift/internal/diff"

// EngineToggle is the ring of diff engines a screen cycles with `e`:
// difftastic (when available) → git's word-diff view → git's moved-line view.
// Without difftastic the ring is just the two git views.
type EngineToggle struct {
	engines []diff.Engine
	idx     int
}

// NewEngineToggle builds the engine ring starting from the given primary
// engine. When the primary already is the plain git engine (difftastic
// unavailable), it isn't added twice.
func NewEngineToggle(engine diff.Engine) EngineToggle {
	engines := []diff.Engine{engine, diff.NewPlainEngine(), diff.NewMovedEngine()}
	if engine.Name() == engines[1].Name() {
		engines = engines[1:]
	}
	return EngineToggle{engines: engines}
}

// Engine is the active engine — what previews are rendered with.
func (e EngineToggle) Engine() diff.Engine { return e.engines[e.idx] }

// Name is the active engine's name, used as the header context label.
func (e EngineToggle) Name() string { return e.Engine().Name() }

// CanToggle reports whether there is more than one engine to cycle through.
func (e EngineToggle) CanToggle() bool { return len(e.engines) > 1 }

// Toggle advances to the next engine in the ring, returning the new state.
func (e EngineToggle) Toggle() EngineToggle {
	e.idx = (e.idx + 1) % len(e.engines)
	return e
}
