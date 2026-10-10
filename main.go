// Package main is the dbman entry point: with no arguments it opens the
// Bubble Tea TUI, otherwise it runs the Cobra CLI.
package main

import (
	"fmt"
	"os"

	"github.com/muizzyranking/dbman/internal/cli"
	"github.com/muizzyranking/dbman/internal/tui"
)

func main() {
	// No args -> open TUI. `dbman tui` also opens it.
	if len(os.Args) == 1 || (len(os.Args) == 2 && os.Args[1] == "tui") {
		if err := tui.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "tui error:", err)
			os.Exit(1)
		}
		return
	}
	root := cli.NewRoot()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
