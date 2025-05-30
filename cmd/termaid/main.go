package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MKlolbullen/termaid/internal/tui"
)

func main() {
	p := tea.NewProgram(
		tui.NewMenu(),       // start at high-level menu
		tea.WithAltScreen(), // use the full terminal
	)

	if err := p.Start(); err != nil {
		log.Fatal(err)
	}
}
