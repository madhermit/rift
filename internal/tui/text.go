package tui

// Truncate shortens s to at most max bytes, appending a trailing ellipsis when
// it overflows. Use for text where the start is most significant (commit
// messages, stash descriptions).
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// TruncatePath shortens s to at most max bytes, prepending a leading ellipsis
// when it overflows. Use for file paths where the end (filename) is most
// significant.
func TruncatePath(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return "..." + s[len(s)-max+3:]
}
