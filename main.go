package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mwtrigg/sdbctl/internal/cli"
	"github.com/mwtrigg/sdbctl/internal/ui"
)

// sdbctl is CLI-first: everything except the editor runs through
// internal/cli, which is also what `sdbctl mcp` exposes to agents. This file
// only decides whether to hand over to the terminal UI.
func main() {
	inv := cli.Main(os.Args[1:], os.Stdout, os.Stderr)

	if inv.LaunchTUI {
		p := tea.NewProgram(
			ui.New(ui.WithESPPath(inv.ESPPath)),
			tea.WithAltScreen(),
		)
		if _, err := p.Run(); err != nil {
			os.Exit(1)
		}
		return
	}
	os.Exit(inv.ExitCode)
}
