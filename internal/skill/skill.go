// Package skill embeds the agent skill that teaches rift's composable review
// workflow, and materializes it to a stable path agents can read.
package skill

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/madhermit/rift/internal/tooling"
)

//go:embed SKILL.md
var content string

// Content returns the skill document.
func Content() string { return content }

// Path materializes the skill to its managed location (under the same data dir
// as the difftastic binary) and returns the absolute path. The file is
// rewritten when its content is stale, so an upgraded binary refreshes it —
// via a temp file and atomic rename, so an agent reading the skill while
// another rift invocation refreshes it never sees a truncated file.
func Path() (string, error) {
	dir, err := tooling.DataDir()
	if err != nil {
		return "", fmt.Errorf("resolve data directory: %w", err)
	}
	p := filepath.Join(dir, "skills", "rift-review", "SKILL.md")
	if existing, err := os.ReadFile(p); err == nil && string(existing) == content {
		return p, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("create skill directory: %w", err)
	}
	if err := tooling.WriteFileAtomic(p, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write skill file: %w", err)
	}
	return p, nil
}
