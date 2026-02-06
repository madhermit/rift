package output

import (
	"bytes"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{
			name: "struct",
			val:  struct{ Name string }{Name: "flux"},
			want: "{\n  \"Name\": \"flux\"\n}\n",
		},
		{
			name: "slice with items",
			val:  []string{"a", "b"},
			want: "[\n  \"a\",\n  \"b\"\n]\n",
		},
		{
			name: "empty slice not null",
			val:  []string{},
			want: "[]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tt.val); err != nil {
				t.Fatalf("WriteJSON() error = %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("WriteJSON() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestWritePlain(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name:  "multiple lines",
			lines: []string{"hello", "world"},
			want:  "hello\nworld\n",
		},
		{
			name:  "empty slice",
			lines: []string{},
			want:  "",
		},
		{
			name:  "nil slice",
			lines: nil,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WritePlain(&buf, tt.lines); err != nil {
				t.Fatalf("WritePlain() error = %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("WritePlain() = %q, want %q", got, tt.want)
			}
		})
	}
}
