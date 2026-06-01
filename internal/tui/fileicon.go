package tui

import (
	"path/filepath"

	"charm.land/lipgloss/v2"
)

// fileType is a glyph plus the ANSI-256 color index it renders in, so files can
// be scanned by language/role at a glance.
type fileType struct {
	icon  string
	color string
}

const genericGlyph = ""

// extFileTypes maps a file extension to its icon and color.
var extFileTypes = map[string]fileType{
	".go":     {"", "45"},
	".js":     {"", "221"},
	".ts":     {"", "39"},
	".jsx":    {"", "221"},
	".tsx":    {"", "39"},
	".py":     {"", "33"},
	".rs":     {"", "173"},
	".rb":     {"", "160"},
	".java":   {"", "167"},
	".c":      {"", "75"},
	".cpp":    {"", "69"},
	".h":      {"", "75"},
	".css":    {"", "75"},
	".html":   {"", "166"},
	".json":   {"", "178"},
	".yaml":   {"", "167"},
	".yml":    {"", "167"},
	".toml":   {"", "173"},
	".md":     {"", "252"},
	".sh":     {"", "71"},
	".lua":    {"", "33"},
	".sql":    {"", "173"},
	".php":    {"", "99"},
	".swift":  {"", "209"},
	".kt":     {"", "141"},
	".dart":   {"", "39"},
	".vue":    {"", "71"},
	".svelte": {"", "202"},
	".zig":    {"", "214"},
	".ex":     {"", "99"},
	".exs":    {"", "99"},
	".hs":     {"", "99"},
	".scala":  {"", "167"},
	".r":      {"", "39"},
}

// nameFileTypes maps a full filename (files identified by name, not extension)
// to its icon and color. Glyphs reuse the proven set above; special files that
// lack a confident glyph are differentiated by color on the generic icon.
var nameFileTypes = map[string]fileType{
	"Dockerfile":        {genericGlyph, "39"},
	"Containerfile":     {genericGlyph, "39"},
	"Makefile":          {genericGlyph, "173"},
	"makefile":          {genericGlyph, "173"},
	"GNUmakefile":       {genericGlyph, "173"},
	"go.mod":            {"", "45"},
	"go.sum":            {"", "45"},
	"go.work":           {"", "45"},
	"package.json":      {"", "71"},
	"package-lock.json": {"", "71"},
	"Cargo.toml":        {"", "173"},
	"Cargo.lock":        {"", "173"},
	"LICENSE":           {genericGlyph, "178"},
	"LICENSE.md":        {genericGlyph, "178"},
	"COPYING":           {genericGlyph, "178"},
	"README":            {"", "252"},
	".gitignore":        {genericGlyph, "240"},
	".gitattributes":    {genericGlyph, "240"},
	".gitmodules":       {genericGlyph, "240"},
	".env":              {genericGlyph, "227"},
}

// FileIcon returns the colored glyph for a path, chosen by filename first then
// extension, falling back to a dim generic document icon.
func FileIcon(path string) string {
	ft, ok := nameFileTypes[filepath.Base(path)]
	if !ok {
		ft, ok = extFileTypes[filepath.Ext(path)]
	}
	if !ok {
		ft = fileType{genericGlyph, "245"} // Subtle
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(ft.color)).Render(ft.icon)
}
