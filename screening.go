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

// Screening is one rote-review run started from the TUI. At most two run at once: one you
// started (s, S, c) and one background tend pass.
type Screening struct {
	Label     string
	StartedAt time.Time
	// Progress holds the stage of each PR rote-review is working on right now.
	Progress map[int]ScreeningProgress
	// PostedUrl is the review rote-review comment posted, if any.
	PostedUrl string
	// Touched is set once rote-review reports working on any PR.
	Touched bool
}

type ScreeningProgress struct {
	Stage       string
	ChunksDone  int
	ChunksTotal int
}

type screeningProgressMsg struct {
	// IsTend marks events from the background tend run, which has its own slot.
	IsTend bool
	Number int
	Event  string
}

type screeningDoneMsg struct {
	IsTend bool
	Err    error
	Detail string
}

// rote-review logs lines like "15:35:04 #85818 own-draft: screening 530 lines in 1 chunk(s)".
var (
	screeningLogLine = regexp.MustCompile(`^\d\d:\d\d:\d\d #(\d+) [a-z-]+: (.*)$`)
	screeningStarted = regexp.MustCompile(`^screening \d+ lines in (\d+) chunk`)
	chunkFinished    = regexp.MustCompile(`^chunk \d+/\d+ →`)
)

// slot returns the field holding the background tend run, or the run you started.
func (model *Model) slot(isTend bool) **Screening {
	if isTend {
		return &model.tending
	}
	return &model.screening
}

// startScreening runs rote-review with arguments in your slot (s, S, c), streaming each
// PR's progress onto the model's event channel. numbers show as starting right away.
func (model *Model) startScreening(label string, numbers []int, arguments ...string) {
	if model.screening != nil {
		model.flash = "rote-review is already running " + model.screening.Label
		return
	}
	model.launchRoteReview(false, label, numbers, arguments)
}

// startTend runs a background `rote-review tend` pass unless one is already running.
func (model *Model) startTend() {
	if model.tending == nil {
		model.lastTendAt = time.Now()
		model.launchRoteReview(true, "tend", nil, []string{"tend"})
	}
}

func (model *Model) launchRoteReview(isTend bool, label string, numbers []int, arguments []string) {
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
	run := &Screening{Label: label, StartedAt: time.Now(), Progress: map[int]ScreeningProgress{}}
	for _, number := range numbers {
		run.Progress[number] = ScreeningProgress{Stage: "start"}
	}
	*model.slot(isTend) = run
	events := model.events
	go func() {
		scanner := bufio.NewScanner(logs)
		detail := ""
		for scanner.Scan() {
			line := scanner.Text()
			if match := screeningLogLine.FindStringSubmatch(line); match != nil {
				number, _ := strconv.Atoi(match[1])
				events <- screeningProgressMsg{IsTend: isTend, Number: number, Event: match[2]}
			} else if strings.TrimSpace(line) != "" {
				detail = line
			}
		}
		events <- screeningDoneMsg{IsTend: isTend, Err: command.Wait(), Detail: detail}
	}()
}

func (model *Model) applyScreeningProgress(message screeningProgressMsg) {
	run := *model.slot(message.IsTend)
	if run == nil {
		return
	}
	run.Touched = true
	progress := run.Progress[message.Number]
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
	case strings.HasPrefix(message.Event, "drafting"):
		progress.Stage = "draft"
	case strings.HasPrefix(message.Event, "posting"):
		progress.Stage = "post"
	case strings.HasPrefix(message.Event, "addressing"):
		progress.Stage = "fix"
	case strings.HasPrefix(message.Event, "marking ready"):
		progress.Stage = "ready"
	case strings.HasPrefix(message.Event, "posted "):
		run.PostedUrl = strings.TrimPrefix(message.Event, "posted ")
		delete(run.Progress, message.Number)
		return
	case strings.HasPrefix(message.Event, "done") || strings.HasPrefix(message.Event, "needs manual review"):
		delete(run.Progress, message.Number)
		model.automations.Reload()
		return
	default:
		return
	}
	if progress.ChunksTotal > 0 && progress.ChunksDone >= progress.ChunksTotal {
		delete(run.Progress, message.Number)
		model.verdicts.Reload()
		return
	}
	run.Progress[message.Number] = progress
}

// finishScreening reports the run's outcome; a background tend pass that did nothing
// stays quiet.
func (model *Model) finishScreening(message screeningDoneMsg) {
	slot := model.slot(message.IsTend)
	run := *slot
	switch {
	case message.Err != nil:
		model.lastError = strings.TrimPrefix(message.Detail, "rote-review: ")
		if model.lastError == "" {
			model.lastError = "rote-review: " + message.Err.Error()
		}
	case run != nil && run.PostedUrl != "":
		model.flash = "posted " + run.PostedUrl
	case run != nil && (!message.IsTend || run.Touched):
		model.flash = fmt.Sprintf("rote-review finished %s in %s", run.Label, formatAge(run.StartedAt))
	}
	*slot = nil
	model.verdicts.Reload()
	model.automations.Reload()
}

// screenSelected checks the selected PR, re-screening a head that already has a verdict.
func (model *Model) screenSelected(pullRequest PullRequest) {
	arguments := []string{"check", strconv.Itoa(pullRequest.Number), "--always"}
	if _, found := model.verdicts.Lookup(pullRequest); found {
		arguments = append(arguments, "--fresh")
	}
	model.startScreening(fmt.Sprintf("#%d", pullRequest.Number), []int{pullRequest.Number}, arguments...)
}

// renderScreeningStatus shows the runs in progress; a background tend pass only once it
// is working on a PR.
func (model Model) renderScreeningStatus() string {
	var parts []string
	for _, run := range []*Screening{model.screening, model.tending} {
		if run != nil && (run != model.tending || len(run.Progress) > 0) {
			parts = append(parts, fmt.Sprintf("rote-review %s %s", run.Label, formatAge(run.StartedAt)))
		}
	}
	return stylePending.Render(strings.Join(parts, " · "))
}

func renderReview(pullRequest PullRequest, verdicts Verdicts, runs ...*Screening) []segment {
	for _, run := range runs {
		if run == nil {
			continue
		}
		if progress, found := run.Progress[pullRequest.Number]; found {
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
