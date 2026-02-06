package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type Mode int

const (
	Interactive Mode = iota
	Print
	JSON
)

func Detect(cmd *cobra.Command) Mode {
	if j, _ := cmd.Flags().GetBool("json"); j {
		return JSON
	}
	if p, _ := cmd.Flags().GetBool("print"); p {
		return Print
	}
	if !IsTerminal() {
		return Print
	}
	return Interactive
}

func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func WritePlain(w io.Writer, lines []string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
