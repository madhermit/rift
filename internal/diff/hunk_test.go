package diff

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestExtractPath covers path resolution across the diff-header forms, including
// git's C-quoted non-ASCII paths and space-disambiguated (trailing-tab) paths,
// which previously matched no branch and silently dropped the whole FileDiff.
func TestExtractPath(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name:  "plain modified",
			lines: []string{`diff --git a/foo.go b/foo.go`, `--- a/foo.go`, `+++ b/foo.go`, `@@ -1 +1 @@`},
			want:  "foo.go",
		},
		{
			name:  "quoted non-ascii path",
			lines: []string{`diff --git "a/caf\303\251.js" "b/caf\303\251.js"`, `--- "a/caf\303\251.js"`, `+++ "b/caf\303\251.js"`, `@@ -1 +1 @@`},
			want:  "café.js",
		},
		{
			name:  "spaced path with trailing tab",
			lines: []string{`diff --git a/my file.js b/my file.js`, "--- a/my file.js\t", "+++ b/my file.js\t", `@@ -1 +1 @@`},
			want:  "my file.js",
		},
		{
			name:  "deleted file uses old side",
			lines: []string{`diff --git a/gone.rb b/gone.rb`, `--- a/gone.rb`, `+++ /dev/null`, `@@ -1 +0 @@`},
			want:  "gone.rb",
		},
		{
			name:  "new file uses new side",
			lines: []string{`diff --git a/new.py b/new.py`, `--- /dev/null`, `+++ b/new.py`, `@@ -0 +1 @@`},
			want:  "new.py",
		},
		{
			name:  "nested a/ prefix not over-stripped",
			lines: []string{`diff --git a/a/b.go b/a/b.go`, `--- a/a/b.go`, `+++ b/a/b.go`, `@@ -1 +1 @@`},
			want:  "a/b.go",
		},
		{
			name:  "quoted header fallback (binary, no +++ lines)",
			lines: []string{`diff --git "a/caf\303\251.png" "b/caf\303\251.png"`, `Binary files a and b differ`},
			want:  "café.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractPath(tt.lines); got != tt.want {
				t.Errorf("extractPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseUnifiedDiffQuotedPath is the end-to-end check that a quoted-path file
// section survives ParseUnifiedDiff with its path and hunks intact.
func TestParseUnifiedDiffQuotedPath(t *testing.T) {
	raw := `diff --git "a/caf\303\251.js" "b/caf\303\251.js"
index 422c2b7..55dce13 100644
--- "a/caf\303\251.js"
+++ "b/caf\303\251.js"
@@ -1,2 +1,2 @@
 a
-b
+B
`
	files := ParseUnifiedDiff(raw)
	if len(files) != 1 {
		t.Fatalf("got %d file diffs, want 1", len(files))
	}
	if files[0].Path != "café.js" {
		t.Errorf("path = %q, want %q", files[0].Path, "café.js")
	}
	if len(files[0].Hunks) != 1 {
		t.Fatalf("got %d hunks, want 1", len(files[0].Hunks))
	}
}

// TestParseUnifiedDiffLarge verifies parsing is linear: the old per-line string
// concatenation was O(n²) and took ~31s on a 60k-line single-file diff. A generous
// bound cleanly separates linear (~tens of ms) from the quadratic regression.
func TestParseUnifiedDiffLarge(t *testing.T) {
	raw := largeDiff(60000)
	start := time.Now()
	files := ParseUnifiedDiff(raw)
	elapsed := time.Since(start)

	if len(files) != 1 {
		t.Fatalf("got %d file diffs, want 1", len(files))
	}
	if len(files[0].Hunks) != 1 {
		t.Fatalf("got %d hunks, want 1", len(files[0].Hunks))
	}
	if elapsed > 2*time.Second {
		t.Fatalf("parsing 60k-line diff took %s, want well under a second (O(n²) regression?)", elapsed)
	}
}

func BenchmarkParseUnifiedDiff(b *testing.B) {
	raw := largeDiff(60000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseUnifiedDiff(raw)
	}
}

// largeDiff builds a single-file unified diff with n added lines.
func largeDiff(n int) string {
	var b strings.Builder
	b.WriteString("diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n")
	fmt.Fprintf(&b, "@@ -1 +1,%d @@\n", n)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "+line %d\n", i)
	}
	return b.String()
}
