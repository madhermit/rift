package diff

import (
	"strings"
	"testing"
)

// The fixtures are difftastic 0.67 output, rendered with --color never.
func TestLayout(t *testing.T) {
	const width = 128
	pad := func(left, right string) string {
		return left + strings.Repeat(" ", 64-len(left)) + right
	}
	tests := []struct {
		name string
		rows []string
		want []Row
	}{
		{
			name: "side by side",
			rows: []string{
				"a.go --- 3/3 --- Go",
				pad("59 func f56()", "60 func f56()"),
				pad("..", "62 "),
				pad("62 func f59()", ".. "),
				pad(" .", " . tail of the line above"),
				"",
			},
			want: []Row{
				{},
				{[]Column{{Start: 3, End: 63}, {Start: 67, End: 128}}},
				{[]Column{{Start: 3, End: 63, Missing: true}, {Start: 67, End: 128}}},
				{[]Column{{Start: 3, End: 63}, {Start: 67, End: 128, Missing: true}}},
				{[]Column{{Start: 3, End: 63, Missing: true}, {Start: 67, End: 128, Continues: true}}},
				{},
			},
		},
		{
			name: "a number per side, one text column",
			rows: []string{"a.go --- 2/3 --- Go", "10 10 func f7()", ".. 11 func onlyRight()", "11 12     indented"},
			want: []Row{{}, {[]Column{{Start: 6, End: width}}}, {[]Column{{Start: 6, End: width}}}, {[]Column{{Start: 6, End: width}}}},
		},
		{
			name: "one number, keeping the code's own indent",
			rows: []string{"a.go --- Go", "  5 func f2() {", "  6     return 2", "  7 "},
			want: []Row{{}, {[]Column{{Start: 4, End: width}}}, {[]Column{{Start: 4, End: width}}}, {[]Column{{Start: 4, End: width}}}},
		},
		{
			name: "inline, where number widths change",
			rows: []string{"a.go --- 2/4 --- Go", "9    func f6()", "10    func f7()", "   11 // new", "2    "},
			want: []Row{
				{},
				{[]Column{{Start: 5, End: width}}},
				{[]Column{{Start: 6, End: width}}},
				{[]Column{{Start: 6, End: width}}},
				{[]Column{{Start: 5, End: width}}},
			},
		},
		{
			name: "a hard-wrapped tail continues the row above",
			rows: []string{"a.go --- Go", "  1 " + strings.Repeat("x", width-4), "tail", "  2 next"},
			want: []Row{{}, {[]Column{{Start: 4, End: width}}}, {[]Column{{End: width, Continues: true}}}, {[]Column{{Start: 4, End: width}}}},
		},
		{
			name: "a note after the header stays plain",
			rows: []string{"a.go --- Go", "File permissions changed from 100600 to 100664.", "  1 package a"},
			want: []Row{{}, {}, {[]Column{{Start: 4, End: width}}}},
		},
		{
			name: "git diff output stays plain",
			rows: []string{"--- a/a.go", "+++ b/a.go", "@@ -1,2 +1,2 @@", "package a", "", "12 monkeys"},
			want: make([]Row, 6),
		},
		{
			name: "text that merely looks like a header stays plain",
			rows: []string{"notes --- draft", "first thought", "12 monkeys", "", "second"},
			want: make([]Row, 5),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Layout(tt.rows, width)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rows, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if len(got[i].Columns) != len(tt.want[i].Columns) {
					t.Fatalf("row %d %q: columns %+v, want %+v", i, tt.rows[i], got[i], tt.want[i])
				}
				for c := range got[i].Columns {
					if got[i].Columns[c] != tt.want[i].Columns[c] {
						t.Errorf("row %d %q column %d: %+v, want %+v", i, tt.rows[i], c, got[i].Columns[c], tt.want[i].Columns[c])
					}
				}
			}
		})
	}
}
