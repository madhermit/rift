package skill

import (
	"os"
	"strings"
	"testing"
)

func TestContent(t *testing.T) {
	c := Content()
	if !strings.HasPrefix(c, "---\nname: rift-review\n") {
		t.Errorf("skill content missing frontmatter header, starts with %q", c[:40])
	}
	if !strings.Contains(c, "rift diff --tests --json") {
		t.Error("skill content missing the tests-lens workflow")
	}
}

func TestPathMaterializes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	p, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("skill file not written: %v", err)
	}
	if string(data) != Content() {
		t.Error("materialized skill differs from embedded content")
	}

	// A stale file (e.g. from an older binary) is refreshed.
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Path(); err != nil {
		t.Fatalf("Path() on stale file: %v", err)
	}
	data, _ = os.ReadFile(p)
	if string(data) != Content() {
		t.Error("stale skill file was not refreshed")
	}
}
