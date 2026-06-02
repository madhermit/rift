package tui

import "github.com/madhermit/rift/internal/diff"

// EngineToggle holds the active diff engine alongside its alternate (the plain
// line engine), letting a screen flip between them with `e`. When only the plain
// engine is available the two sides match and the flip is a no-op.
type EngineToggle struct {
	active    diff.Engine
	alt       diff.Engine
	canToggle bool // the pair is fixed, so toggle-ability is computed once
}

// NewEngineToggle pairs the given engine with the plain line engine as its
// alternate.
func NewEngineToggle(engine diff.Engine) EngineToggle {
	alt := diff.NewPlainEngine()
	return EngineToggle{active: engine, alt: alt, canToggle: engine.Name() != alt.Name()}
}

// Engine is the active engine — what previews are rendered with.
func (e EngineToggle) Engine() diff.Engine { return e.active }

// Name is the active engine's name, used as the header context label.
func (e EngineToggle) Name() string { return e.active.Name() }

// CanToggle reports whether the two engines differ (false when only the plain
// engine is available).
func (e EngineToggle) CanToggle() bool { return e.canToggle }

// Toggle swaps the active and alternate engines, returning the new state; a
// no-op when only one engine is available.
func (e EngineToggle) Toggle() EngineToggle {
	if e.canToggle {
		e.active, e.alt = e.alt, e.active
	}
	return e
}
