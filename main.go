package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pdrolopes/syncthing_TUI/app"
)

func main() {
	zone.NewGlobal()
	p := tea.NewProgram(app.NewModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
