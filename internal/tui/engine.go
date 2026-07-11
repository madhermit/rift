package tui

import "github.com/madhermit/rift/internal/diff"

// EngineToggle is the ring of diff engines a screen cycles with `e`:
// difftastic (when available) → git's word-diff view → git's moved-line view.
type EngineToggle struct {
	engines []diff.Engine
	idx     int
}

// NewEngineToggle builds the engine ring starting from the given primary
// engine. Alternates matching the primary aren't added twice, so a ring seeded
// with an already-cycled git engine (a log drilldown passes the active engine)
// stays duplicate-free. The moved-line view is only offered when color is on —
// its flags are inert without color, which would make it indistinguishable
// from the plain view.
func NewEngineToggle(engine diff.Engine) EngineToggle {
	alts := []diff.Engine{diff.NewPlainEngine()}
	if ColorEnabled() {
		alts = append(alts, diff.NewMovedEngine())
	}
	return EngineToggle{engines: ring(engine, alts)}
}

// NewHunkEngineToggle is the ring for hunk-rendering screens (stage): the
// moved-line view is omitted because DiffHunks renders hunks directly, where
// git's move detection never engages — it would cycle identical output.
func NewHunkEngineToggle(engine diff.Engine) EngineToggle {
	return EngineToggle{engines: ring(engine, []diff.Engine{diff.NewPlainEngine()})}
}

func ring(primary diff.Engine, alts []diff.Engine) []diff.Engine {
	engines := []diff.Engine{primary}
	for _, alt := range alts {
		if alt.Name() != primary.Name() {
			engines = append(engines, alt)
		}
	}
	return engines
}

// Engine is the active engine — what previews are rendered with.
func (e EngineToggle) Engine() diff.Engine { return e.engines[e.idx] }

// Name is the active engine's name, used as the header context label.
func (e EngineToggle) Name() string { return e.Engine().Name() }

// CanToggle reports whether there is more than one engine to cycle through
// (false e.g. without difftastic under NO_COLOR — the hint is then hidden).
func (e EngineToggle) CanToggle() bool { return len(e.engines) > 1 }

// Toggle advances to the next engine in the ring, returning the new state; a
// no-op when there's nothing to cycle to.
func (e EngineToggle) Toggle() EngineToggle {
	if len(e.engines) > 1 {
		e.idx = (e.idx + 1) % len(e.engines)
	}
	return e
}
