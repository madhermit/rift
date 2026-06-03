package reviewui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madhermit/rift/internal/review"
)

func TestSpecRow(t *testing.T) {
	spec := review.Spec{
		Path:   []string{"chain scope"},
		Name:   "keeps own rows",
		Status: "added",
		Ticket: "ENG-2790",
	}

	// Wide enough: the ticket is right-aligned at the edge.
	wide := ansi.Strip(specRow(spec, 60, false))
	if !strings.Contains(wide, "chain scope › keeps own rows") {
		t.Errorf("missing name/path: %q", wide)
	}
	if !strings.HasSuffix(wide, "ENG-2790") {
		t.Errorf("ticket not right-aligned: %q", wide)
	}
	if lipgloss.Width(specRow(spec, 60, false)) > 60 {
		t.Errorf("row exceeds width 60")
	}

	// Narrow: must not panic and must stay within the width (ticket dropped).
	narrow := specRow(spec, 8, false)
	if lipgloss.Width(narrow) > 8 {
		t.Errorf("narrow row exceeds width 8: %q", ansi.Strip(narrow))
	}
}
