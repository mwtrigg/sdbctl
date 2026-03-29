package main

import (
	"flag"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mwtrigg/sdboot-tui/internal/ui"
)

func main() {
	espPath := flag.String("esp", "", "Path to ESP mount point (e.g., /boot, /boot/efi, or custom)")
	flag.Parse()

	p := tea.NewProgram(
		ui.New(ui.WithESPPath(*espPath)),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}
