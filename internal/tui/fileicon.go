package tui

import "path/filepath"

var fileIcons = map[string]string{
	".go":     "\ue627",
	".js":     "\ue74e",
	".ts":     "\ue628",
	".jsx":    "\ue7ba",
	".tsx":    "\ue7ba",
	".py":     "\ue73c",
	".rs":     "\ue7a8",
	".rb":     "\ue739",
	".java":   "\ue738",
	".c":      "\ue61e",
	".cpp":    "\ue61d",
	".h":      "\ue61e",
	".css":    "\ue749",
	".html":   "\ue736",
	".json":   "\ue60b",
	".yaml":   "\uf481",
	".yml":    "\uf481",
	".toml":   "\ue6b2",
	".md":     "\ue73e",
	".sh":     "\ue795",
	".lua":    "\ue620",
	".sql":    "\ue706",
	".php":    "\ue73d",
	".swift":  "\ue755",
	".kt":     "\ue634",
	".dart":   "\ue798",
	".vue":    "\ue6a0",
	".svelte": "\ue6aa",
	".zig":    "\ue6a9",
	".ex":     "\ue62d",
	".exs":    "\ue62d",
	".hs":     "\ue61f",
	".scala":  "\ue737",
	".r":      "\ue68a",
}

func FileIcon(path string) string {
	if icon, ok := fileIcons[filepath.Ext(path)]; ok {
		return icon
	}
	return "\uf016"
}
