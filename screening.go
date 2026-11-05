package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Screening is one rote-review run started from the TUI; only one runs at a time,
// since rote-review itself holds an exclusive lock.
type Screening struct {
	Label     string
	StartedAt time.Time
	// Progress holds the stage of each PR rote-review is working on right now.
	Progress map[int]ScreeningProgress
}

type ScreeningProgress struct {
	Stage       string
	ChunksDone  int
	ChunksTotal int
}

type screeningProgressMsg struct {
	Number int
	Event  string
}

type screeningDoneMsg struct {
	Err    error
	Detail string
}

// rote-review logs lines like "15:35:04 #85818 own-draft: screening 530 lines in 1 chunk(s)".
var (
	screeningLogLine = regexp.MustCompile(`^\d\d:\d\d:\d\d #(\d+) [a-z-]+: (.*)$`)
	screeningStarted = regexp.MustCompile(`^screening \d+ lines in (\d+) chunk`)
	chunkFinished    = regexp.MustCompile(`^chunk \d+/\d+ →`)
)

// startScreening runs rote-review with arguments and streams each PR's progress onto
// the model's event channel. numbers are marked as starting before rote-review logs.
func (model *Model) startScreening(label string, numbers []int, arguments ...string) {
	if model.screening != nil {
		model.flash = "rote-review is already running " + model.screening.Label
		return
	}
	command := exec.Command("rote-review", append(arguments, "--repo", model.repository, "--checkout", model.checkout)...)
	logs, pipeError := command.StderrPipe()
	if pipeError != nil {
		model.lastError = pipeError.Error()
		return
	}
	if startError := command.Start(); startError != nil {
		model.lastError = "rote-review: " + startError.Error()
		return
	}
	model.screening = &Screening{Label: label, StartedAt: time.Now(), Progress: map[int]ScreeningProgress{}}
	for _, number := range numbers {
		model.screening.Progress[number] = ScreeningProgress{Stage: "start"}
	}
	events := model.events
	go func() {
		scanner := bufio.NewScanner(logs)
		detail := ""
		for scanner.Scan() {
			line := scanner.Text()
			if match := screeningLogLine.FindStringSubmatch(line); match != nil {
				number, _ := strconv.Atoi(match[1])
				events <- screeningProgressMsg{Number: number, Event: match[2]}
			} else if strings.TrimSpace(line) != "" {
				detail = line
			}
		}
		events <- screeningDoneMsg{Err: command.Wait(), Detail: detail}
	}()
}

func (model *Model) applyScreeningProgress(message screeningProgressMsg) {
	if model.screening == nil {
		return
	}
	progress := model.screening.Progress[message.Number]
	switch {
	case strings.HasPrefix(message.Event, "fetching"):
		progress = ScreeningProgress{Stage: "fetch"}
	case strings.HasPrefix(message.Event, "checking out"):
		progress.Stage = "checkout"
	case screeningStarted.MatchString(message.Event):
		total, _ := strconv.Atoi(screeningStarted.FindStringSubmatch(message.Event)[1])
		progress = ScreeningProgress{Stage: "screen", ChunksTotal: total}
	case chunkFinished.MatchString(message.Event):
		progress.ChunksDone++
	default:
		return
	}
	if progress.ChunksTotal > 0 && progress.ChunksDone >= progress.ChunksTotal {
		delete(model.screening.Progress, message.Number)
		model.verdicts.Reload()
		return
	}
	model.screening.Progress[message.Number] = progress
}

func (model *Model) finishScreening(message screeningDoneMsg) {
	if message.Err != nil {
		model.lastError = strings.TrimPrefix(message.Detail, "rote-review: ")
		if model.lastError == "" {
			model.lastError = "rote-review: " + message.Err.Error()
		}
	} else if model.screening != nil {
		model.flash = fmt.Sprintf("rote-review finished %s in %s", model.screening.Label, formatAge(model.screening.StartedAt))
	}
	model.screening = nil
	model.verdicts.Reload()
}

// screenSelected checks the selected PR, re-screening a head that already has a verdict.
func (model *Model) screenSelected(pullRequest PullRequest) {
	arguments := []string{"check", strconv.Itoa(pullRequest.Number), "--always"}
	if _, found := model.verdicts.Lookup(pullRequest); found {
		arguments = append(arguments, "--fresh")
	}
	model.startScreening(fmt.Sprintf("#%d", pullRequest.Number), []int{pullRequest.Number}, arguments...)
}

func (model Model) renderScreeningStatus() string {
	if model.screening == nil {
		return ""
	}
	return stylePending.Render(fmt.Sprintf("rote-review %s %s", model.screening.Label, formatAge(model.screening.StartedAt)))
}

func renderReview(pullRequest PullRequest, verdicts Verdicts, screening *Screening) []segment {
	if screening != nil {
		if progress, found := screening.Progress[pullRequest.Number]; found {
			if progress.ChunksTotal > 0 {
				return []segment{{fmt.Sprintf("⟳ %d/%d", progress.ChunksDone, progress.ChunksTotal), stylePending}}
			}
			return []segment{{"⟳ " + progress.Stage, stylePending}}
		}
	}
	if verdict, found := verdicts.Lookup(pullRequest); found && pullRequest.isOpen() {
		// Only a rote PR is held back from auto-approval; the detail pane shows any hold.
		if len(verdict.Held) > 0 && verdict.Attention == "rote" {
			return []segment{{verdict.Attention, attentionStyle(verdict.Attention)}, {"·held", styleHeld}}
		}
		return []segment{{verdict.Attention, attentionStyle(verdict.Attention)}}
	}
	return nil
}
