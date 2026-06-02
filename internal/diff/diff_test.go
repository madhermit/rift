package diff

import (
	"fmt"
	"slices"
	"testing"
)

// TestParallelStreamOrdered verifies results stream in index order and the
// channel closes — even when later items finish before earlier ones.
func TestParallelStreamOrdered(t *testing.T) {
	gate := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	ch := ParallelStream(len(gate), func(i int) string {
		<-gate[i]
		return fmt.Sprintf("r%d", i)
	})
	// Finish in reverse order; the output order must still be r0, r1, r2.
	close(gate[2])
	close(gate[1])
	close(gate[0])

	var got []string
	for s := range ch {
		got = append(got, s)
	}
	if want := []string{"r0", "r1", "r2"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildGitDiffArgs(t *testing.T) {
	tests := []struct {
		name    string
		opts    DiffOpts
		file    string
		display bool
		want    []string
	}{
		{
			name: "staged with color (no display flags)",
			opts: DiffOpts{Staged: true, Color: true},
			file: "main.go",
			want: []string{"diff", "--color=always", "--staged", "--", "main.go"},
		},
		{
			name: "staged no color",
			opts: DiffOpts{Staged: true, Color: false},
			file: "main.go",
			want: []string{"diff", "--color=never", "--staged", "--", "main.go"},
		},
		{
			name: "base only",
			opts: DiffOpts{Base: "HEAD~1"},
			file: "main.go",
			want: []string{"diff", "--color=never", "HEAD~1", "--", "main.go"},
		},
		{
			name: "base and target",
			opts: DiffOpts{Base: "abc123", Target: "def456"},
			file: "main.go",
			want: []string{"diff", "--color=never", "abc123", "def456", "--", "main.go"},
		},
		{
			name: "no opts (working tree)",
			opts: DiffOpts{},
			file: "main.go",
			want: []string{"diff", "--color=never", "--", "main.go"},
		},
		{
			name: "color with base and target",
			opts: DiffOpts{Base: "a", Target: "b", Color: true},
			file: "f.go",
			want: []string{"diff", "--color=always", "a", "b", "--", "f.go"},
		},
		{
			name: "empty file omits separator",
			opts: DiffOpts{Color: true},
			file: "",
			want: []string{"diff", "--color=always"},
		},
		{
			name: "empty file with staged",
			opts: DiffOpts{Staged: true},
			file: "",
			want: []string{"diff", "--color=never", "--staged"},
		},
		{
			name:    "display with color adds word-diff and ws flags",
			opts:    DiffOpts{Staged: true, Color: true},
			file:    "main.go",
			display: true,
			want:    []string{"diff", "--color=always", "--word-diff=color", "--ws-error-highlight=all", "--staged", "--", "main.go"},
		},
		{
			name:    "display without color stays vanilla",
			opts:    DiffOpts{Staged: true, Color: false},
			file:    "main.go",
			display: true,
			want:    []string{"diff", "--color=never", "--staged", "--", "main.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGitDiffArgs(tt.opts, tt.file, tt.display)
			if !slices.Equal(got, tt.want) {
				t.Errorf("buildGitDiffArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewEngine(t *testing.T) {
	engine := NewEngine()
	name := engine.Name()
	if name != "difftastic" && name != "git-diff" {
		t.Errorf("NewEngine().Name() = %q, want \"difftastic\" or \"git-diff\"", name)
	}
}
