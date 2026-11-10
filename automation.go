package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// tendInterval is how often review-queue runs a background `rote-review tend` pass.
const tendInterval = 3 * time.Minute

// Automation mirrors rote-review's feedback-loop state for one of your own PRs.
type Automation struct {
	Status     string    `json:"status"`
	Loops      int       `json:"loops"`
	LastResult string    `json:"lastResult"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Automations reads the automation.json rote-review writes next to its verdicts.
type Automations struct {
	path       string
	modifiedAt time.Time
	byNumber   map[int]Automation
}

func NewAutomations(repository string) Automations {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}
	return Automations{path: filepath.Join(stateHome, "rote-review", strings.ReplaceAll(repository, "/", "-"), "automation.json"), byNumber: map[int]Automation{}}
}

// Reload rereads the file only when rote-review has rewritten it.
func (automations *Automations) Reload() {
	info, statError := os.Stat(automations.path)
	if statError != nil || info.ModTime().Equal(automations.modifiedAt) {
		return
	}
	contents, readError := os.ReadFile(automations.path)
	byNumber := map[int]Automation{}
	if readError != nil || json.Unmarshal(contents, &byNumber) != nil {
		return
	}
	automations.modifiedAt, automations.byNumber = info.ModTime(), byNumber
}

// Lookup defaults to automatic, which is how rote-review treats a PR it has not touched.
func (automations Automations) Lookup(number int) Automation {
	if automation, found := automations.byNumber[number]; found {
		return automation
	}
	return Automation{Status: "auto"}
}

func renderAutomation(automation Automation) segment {
	switch {
	case automation.Status == "manual":
		return segment{"manual", styleHeld}
	case automation.Loops > 0:
		return segment{fmt.Sprintf("auto %d/5", automation.Loops), stylePending}
	default:
		return segment{"auto", styleDim}
	}
}

type automationChangedMsg struct {
	Text     string
	Err      error
	IsResume bool
}

// changeAutomation runs `rote-review resume` or `rote-review draft` on one of your PRs.
func (model Model) changeAutomation(command string, number int) tea.Cmd {
	return func() tea.Msg {
		output, runError := exec.Command("rote-review", command, strconv.Itoa(number), "--repo", model.repository).CombinedOutput()
		if runError != nil {
			return automationChangedMsg{Err: fmt.Errorf("rote-review %s: %s", command, strings.TrimSpace(string(output)))}
		}
		text := fmt.Sprintf("#%d moved to draft; automation paused", number)
		if command == "resume" {
			text = fmt.Sprintf("#%d automation resumed", number)
		}
		return automationChangedMsg{Text: text, IsResume: command == "resume"}
	}
}

func renderAutomationDetail(automation Automation, width int) []string {
	badge := renderAutomation(automation)
	text := badge.style.Render(badge.text) + " automation"
	if automation.LastResult != "" {
		text += ": " + automation.LastResult + styleDim.Render(" ("+formatAge(automation.UpdatedAt)+" ago)")
	}
	if automation.Status == "manual" {
		text += styleDim.Render(" · a resumes, D moves to draft")
	}
	return strings.Split(ansi.Wrap(text, width, ""), "\n")
}
