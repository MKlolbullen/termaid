package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MKlolbullen/termaid/internal/tui"
)

func main() {
	prog := tea.NewProgram(
		tui.NewMenu(),
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(), // ← mouse support
	)

	if err := prog.Start(); err != nil {
		log.Fatal(err)
	}
}
