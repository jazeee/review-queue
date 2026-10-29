package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	repository := flag.String("repo", "VivaaHealth/vivaa", "GitHub repository (owner/name) to watch.")
	home, _ := os.UserHomeDir()
	checkout := flag.String("checkout", filepath.Join(home, "code", "notable", "vivaa"), "Local clone of the repository, used by p to create worktrees.")
	days := flag.Int("days", 14, "How many days of review requests to keep.")
	flag.Parse()

	program := tea.NewProgram(
		NewModel(*repository, *checkout, time.Duration(*days)*24*time.Hour),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithReportFocus(),
	)
	if _, runError := program.Run(); runError != nil {
		fmt.Fprintln(os.Stderr, "review-queue exited with error:", runError)
		os.Exit(1)
	}
}
