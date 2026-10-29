package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

var terminalPreference = []string{
	"wezterm", "ghostty", "kitty", "alacritty", "foot", "ptyxis",
	"gnome-terminal", "konsole", "tilix", "terminator", "xterm",
	"x-terminal-emulator",
}

func buildTerminalCommand(terminalName string, workingDirectory string, innerCommand []string) []string {
	var argv []string
	separator := "-e"
	switch terminalName {
	case "wezterm":
		argv, separator = []string{"wezterm", "start", "--cwd", workingDirectory}, "--"
	case "ghostty":
		argv = []string{"ghostty", "--working-directory=" + workingDirectory}
	case "kitty":
		return append([]string{"kitty", "--directory", workingDirectory}, innerCommand...)
	case "alacritty":
		argv = []string{"alacritty", "--working-directory", workingDirectory}
	case "ptyxis":
		argv, separator = []string{"ptyxis", "--new-window", "--working-directory=" + workingDirectory}, "--"
	case "gnome-terminal":
		argv, separator = []string{"gnome-terminal", "--working-directory=" + workingDirectory}, "--"
	case "konsole":
		argv = []string{"konsole", "--workdir", workingDirectory}
	case "tilix":
		argv = []string{"tilix", "--working-directory", workingDirectory}
	case "terminator":
		// terminator's -e takes a single string.
		return []string{"terminator", "--working-directory", workingDirectory, "-e", strings.Join(innerCommand, " ")}
	default:
		argv = []string{terminalName}
	}
	return append(append(argv, separator), innerCommand...)
}

func launchDetachedProcess(argv []string, workingDirectory string) error {
	if len(argv) == 0 {
		return errors.New("no command to launch")
	}
	command := exec.Command(argv[0], argv[1:]...)
	command.Dir = workingDirectory
	devNull, openError := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if openError == nil {
		defer devNull.Close()
		command.Stdin, command.Stdout, command.Stderr = devNull, devNull, devNull
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if startError := command.Start(); startError != nil {
		return startError
	}
	go command.Wait()
	return nil
}

func OpenUrl(url string) error {
	return launchDetachedProcess([]string{"xdg-open", url}, "")
}

// LaunchPiInTerminal opens a new terminal window running the pi tmux launcher in
// directory, preferring $TERMINAL. It returns the terminal used.
func LaunchPiInTerminal(directory string) (string, error) {
	home, _ := os.UserHomeDir()
	launcher := filepath.Join(home, ".local", "bin", "pi-worktree-session.sh")
	if _, statError := os.Stat(launcher); statError != nil {
		return "", errors.New("pi launcher not found: " + launcher)
	}
	candidates := terminalPreference
	if environmentTerminal := os.Getenv("TERMINAL"); environmentTerminal != "" {
		candidates = append([]string{environmentTerminal}, terminalPreference...)
	}
	for _, terminalName := range candidates {
		if _, lookupError := exec.LookPath(terminalName); lookupError != nil {
			continue
		}
		argv := buildTerminalCommand(terminalName, directory, []string{launcher, directory})
		// ptyxis sizes a new window from the monitor it is born on; this KWin helper spawns it on the largest screen.
		if helper := filepath.Join(home, ".local", "bin", "ptyxis-new-window.sh"); terminalName == "ptyxis" {
			if _, statError := os.Stat(helper); statError == nil {
				argv = append([]string{helper}, argv...)
			}
		}
		return terminalName, launchDetachedProcess(argv, directory)
	}
	return "", errors.New("no terminal emulator found (set $TERMINAL)")
}
