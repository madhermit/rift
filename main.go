package main

import (
	"os"

	"github.com/madhermit/rift/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
