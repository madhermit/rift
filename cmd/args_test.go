package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestPathsOnly(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"nothing", nil, false},
		{"paths after dash", []string{"--", "a.go"}, false},
		{"bare argument", []string{"a.go"}, true},
		{"bare argument before dash", []string{"a.go", "--", "b.go"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "x", Args: pathsOnly, RunE: func(*cobra.Command, []string) error { return nil }, SilenceErrors: true, SilenceUsage: true}
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
