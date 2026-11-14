package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mwtrigg/sdboot-tui/internal/boot"
	"github.com/mwtrigg/sdboot-tui/internal/ui"
)

const usage = `sdboot-tui — terminal UI for systemd-boot

Usage:
  sdboot-tui [options]            start the TUI
  sdboot-tui --check              print a diagnostics report and exit
  sdboot-tui --restore            restore the most recent backup and exit
  sdboot-tui --list-backups       list available backups and exit

Options:
  -esp PATH   boot directory to manage (default: autodetected /boot, /boot/efi, /efi)
  -check      run diagnostics without the UI; exit 1 if problems were found
  -restore    restore the most recent snapshot and exit
  -list-backups
              list snapshots and exit
  -version    print the version and exit
`

func main() {
	espPath := flag.String("esp", "", "Path to ESP mount point (e.g., /boot, /boot/efi, or custom)")
	check := flag.Bool("check", false, "Print a diagnostics report and exit (non-zero exit if problems are found)")
	restore := flag.Bool("restore", false, "Restore the most recent backup and exit")
	listBackups := flag.Bool("list-backups", false, "List backups and exit")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if *showVersion {
		fmt.Println("sdboot-tui", ui.AppVersion)
		return
	}

	if *check || *restore || *listBackups {
		os.Exit(runCli(*espPath, *check, *restore, *listBackups))
	}

	p := tea.NewProgram(
		ui.New(ui.WithESPPath(*espPath)),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}

// runCli implements the non-interactive modes. It returns the process exit
// code: 0 = fine, 1 = problems found (check) or operation failed, 2 = usage
// error (e.g. no boot directory found).
func runCli(espPath string, check, restore, listBackups bool) int {
	dir := espPath
	if dir == "" {
		found, err := boot.FindESP()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
		dir = found
	}

	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "error: %s is not a directory\n", dir)
		return 2
	}

	switch {
	case listBackups:
		backups, err := boot.ListBackups(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if len(backups) == 0 {
			fmt.Println("no backups found in", boot.BackupRoot(dir))
			return 0
		}
		fmt.Println("backups in", boot.BackupRoot(dir))
		for _, b := range backups {
			fmt.Println(" ", b.Describe())
		}
		return 0

	case restore:
		b, err := boot.LatestBackup(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if b == nil {
			fmt.Fprintln(os.Stderr, "error: no backups to restore")
			return 1
		}
		// Snapshot the current state first, so the restore is reversible.
		if _, err := boot.CreateBackup(dir, b.Files); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if err := boot.RestoreBackup(dir, *b); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println("restored", b.Describe())
		return 0

	case check:
		report, err := boot.BuildReport(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
		fmt.Print(report.Text())
		if !report.OK() {
			return 1
		}
		return 0
	}
	return 0
}
