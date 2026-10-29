package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func (result CommandResult) Succeeded() bool {
	return result.ExitCode == 0
}

// RunCommand blocks until the process exits; ExitCode is 124 on timeout, 127 when missing.
func RunCommand(timeout time.Duration, workingDirectory string, name string, arguments ...string) CommandResult {
	commandContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := exec.CommandContext(commandContext, name, arguments...)
	command.Dir = workingDirectory
	var standardOutput, standardError strings.Builder
	command.Stdout = &standardOutput
	command.Stderr = &standardError

	runError := command.Run()
	result := CommandResult{Stdout: standardOutput.String(), Stderr: standardError.String()}
	if commandContext.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		result.Stderr = "timeout after " + timeout.String()
		return result
	}
	var notFound *exec.Error
	var exitError *exec.ExitError
	switch {
	case runError == nil:
	case errors.As(runError, &notFound):
		result.ExitCode = 127
		result.Stderr = runError.Error()
	case errors.As(runError, &exitError):
		result.ExitCode = exitError.ExitCode()
	default:
		result.ExitCode = 1
	}
	return result
}

func OpenUrl(url string) error {
	command := exec.Command("xdg-open", url)
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

// CopyToSystemClipboard discards tool output because wl-copy forks a daemon that would hold the pipe open.
func CopyToSystemClipboard(text string) error {
	for _, candidate := range [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
		{"pbcopy"},
	} {
		if _, lookupError := exec.LookPath(candidate[0]); lookupError != nil {
			continue
		}
		clipboardContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		command := exec.CommandContext(clipboardContext, candidate[0], candidate[1:]...)
		command.Stdin = strings.NewReader(text)
		return command.Run()
	}
	return errors.New("no clipboard tool found (install wl-clipboard / xclip / xsel)")
}
