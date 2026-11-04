package diff

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPlainReason(t *testing.T) {
	tests := []struct {
		name, out string
		want      string
	}{
		{"no parser", "b.md --- Text\n1 # T\n", "difftastic has no parser for this file"},
		{"no parser, one hunk of several", "b.txt --- 2/3 --- Text\n1 a\n", "difftastic has no parser for this file"},
		{"too large", "big.json --- Text (2.6 MiB exceeded DFT_BYTE_LIMIT)\n1 [\n", "too large for difftastic"},
		{"too complex", "d.go --- Text (exceeded DFT_GRAPH_LIMIT)\n1 a\n", "too complex for difftastic"},
		{"broken syntax", "b.go --- Text (2 Go parse errors, exceeded DFT_PARSE_ERROR_LIMIT)\n1 a\n", "difftastic couldn't parse this file"},
		{"formatting only", "d.go --- Go\nNo syntactic changes.\n", "difftastic found no syntactic changes"},
		{"formatting only, after a note", "d.go --- Go\nFile permissions changed from 100600 to 100664.\nNo syntactic changes.\n", "difftastic found no syntactic changes"},
		{"colored header", "\x1b[1m\x1b[93mb.md\x1b[39m\x1b[0m\x1b[2m --- Text\x1b[0m\n1 a\n", "difftastic has no parser for this file"},
		{"parsed code", "a.go --- Go\n1 package a\n", ""},
		{"parsed code, one hunk of several", "a.go --- 1/2 --- Go\n1 package a\n", ""},
		{"a path that looks like a fallback", "x --- Text.go --- Go\n1 package a\n", ""},
		{"the message as code further down", "a.go --- Go\n1 a\n2 b\nNo syntactic changes.\n", ""},
		{"a rendered hunk with no header", "@@ -1 +1 @@\n-a\n+b\n", ""},
		{"nothing", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plainReason(tt.out); got != tt.want {
				t.Errorf("plainReason = %q, want %q", got, tt.want)
			}
		})
	}
}

type fakeEngine struct {
	name  string
	out   string
	err   error
	hunks []string
}

func (f fakeEngine) Name() string { return f.name }
func (f fakeEngine) Diff(context.Context, string, File, DiffOpts) (string, error) {
	return f.out, f.err
}
func (f fakeEngine) DiffHunks(context.Context, []Hunk, string, string, bool, int) []string {
	return f.hunks
}

func TestAutoDiffChoosesPerFile(t *testing.T) {
	const parsed, textual, gitOut = "a.go --- Go\n1 package a\n", "b.md --- Text\n1 # T\n", "diff --git a/b.md b/b.md\n"
	boom := errors.New("boom")
	tests := []struct {
		name       string
		structural fakeEngine
		plain      fakeEngine
		want       string
		wantErr    bool
	}{
		{name: "parsed stays structural", structural: fakeEngine{out: parsed}, plain: fakeEngine{out: gitOut}, want: parsed},
		{name: "fallback goes to git, with why", structural: fakeEngine{out: textual}, plain: fakeEngine{out: gitOut},
			want: "git-diff: difftastic has no parser for this file\n" + gitOut},
		{name: "git has nothing: keep difftastic", structural: fakeEngine{out: textual}, plain: fakeEngine{out: " \n"}, want: textual},
		{name: "git fails: keep difftastic", structural: fakeEngine{out: textual}, plain: fakeEngine{err: boom}, want: textual},
		{name: "difftastic failure surfaces", structural: fakeEngine{err: boom}, plain: fakeEngine{out: gitOut}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &autoEngine{structural: tt.structural, plain: tt.plain}
			got, err := a.Diff(context.Background(), "", File{}, DiffOpts{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Diff = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAutoNoteIsDimmedOnlyWithColor(t *testing.T) {
	a := &autoEngine{structural: fakeEngine{out: "b.md --- Text\n"}, plain: fakeEngine{out: "diff\n"}}
	got, _ := a.Diff(context.Background(), "", File{}, DiffOpts{Color: true})
	if !strings.HasPrefix(got, "\x1b[2mgit-diff: ") {
		t.Errorf("colored note should be dim: %q", got)
	}
}

func TestAutoDiffHunksChoosesPerHunk(t *testing.T) {
	a := &autoEngine{
		structural: fakeEngine{hunks: []string{"a.go --- Go\n1 x\n", "a.go --- Go\nNo syntactic changes.\n"}},
		plain:      fakeEngine{hunks: []string{"@@ one", "@@ two"}},
	}
	got := a.DiffHunks(context.Background(), make([]Hunk, 2), "a.go", "", false, 80)
	if got[0] != "a.go --- Go\n1 x\n" || got[1] != "@@ two" {
		t.Errorf("DiffHunks = %q", got)
	}
}

func TestAlternates(t *testing.T) {
	names := func(es []Engine) string {
		var out []string
		for _, e := range es {
			out = append(out, e.Name())
		}
		return strings.Join(out, ",")
	}
	structural, plain := fakeEngine{name: "difftastic"}, NewPlainEngine()
	tests := []struct {
		engine Engine
		want   string
	}{
		{&autoEngine{structural: structural, plain: plain}, "difftastic,git-diff"},
		{structural, "git-diff"},
		{plain, ""},
	}
	for _, tt := range tests {
		if got := names(Alternates(tt.engine)); got != tt.want {
			t.Errorf("Alternates(%s) = %q, want %q", tt.engine.Name(), got, tt.want)
		}
	}
}
