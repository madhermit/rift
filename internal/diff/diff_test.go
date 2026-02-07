package diff

import (
	"slices"
	"testing"
)

func TestBuildGitDiffArgs(t *testing.T) {
	tests := []struct {
		name string
		opts DiffOpts
		file string
		want []string
	}{
		{
			name: "staged with color",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGitDiffArgs(tt.opts, tt.file)
			if !slices.Equal(got, tt.want) {
				t.Errorf("buildGitDiffArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildCommitDiffArgs(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		target string
		color  bool
		want   []string
	}{
		{
			name:   "with color",
			base:   "abc",
			target: "def",
			color:  true,
			want:   []string{"diff", "--color=always", "abc..def"},
		},
		{
			name:   "no color",
			base:   "abc",
			target: "def",
			color:  false,
			want:   []string{"diff", "--color=never", "abc..def"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCommitDiffArgs(tt.base, tt.target, tt.color)
			if !slices.Equal(got, tt.want) {
				t.Errorf("buildCommitDiffArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"whitespace only", "  \n  ", nil},
		{"single line", "foo.go\n", []string{"foo.go"}},
		{"multiple lines", "a.go\nb.go\nc.go\n", []string{"a.go", "b.go", "c.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if !slices.Equal(got, tt.want) {
				t.Errorf("splitLines(%q) = %v, want %v", tt.input, got, tt.want)
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
