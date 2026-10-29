package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	repository := flag.String("repo", "VivaaHealth/vivaa", "GitHub repository (owner/name) to watch.")
	days := flag.Int("days", 14, "How many days of review requests to keep.")
	flag.Parse()

	program := tea.NewProgram(
		NewModel(*repository, time.Duration(*days)*24*time.Hour),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, runError := program.Run(); runError != nil {
		fmt.Fprintln(os.Stderr, "review-queue exited with error:", runError)
		os.Exit(1)
	}
}
