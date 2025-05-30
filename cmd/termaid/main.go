package main

import (
	"log"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/MKlolbullen/termaid/internal/tui"
)

func main() {
	// Launch the TUI in full-screen “AltScreen” mode.
	prog := tea.NewProgram(
		tui.NewMenu(),      // entry point = main menu
		tea.WithAltScreen(), // use the whole terminal
	)

	if err := prog.Start(); err != nil {
		log.Fatal(err)
	}
}
