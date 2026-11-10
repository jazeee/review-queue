package main

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

// startGuidance opens the guidance field for posting the PR's rote-review feedback, once
// the PR has a verdict for its current head to agree with.
func (model *Model) startGuidance(pullRequest PullRequest) {
	if _, found := model.verdicts.Lookup(pullRequest); !found {
		model.flash = fmt.Sprintf("#%d has no rote-review verdict for its current head; press s first", pullRequest.Number)
		return
	}
	model.writingGuidance = true
	model.guidanceNumber = pullRequest.Number
	model.guidanceInput.SetValue("")
	model.guidanceInput.Focus()
}

func (model Model) updateGuidance(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "ctrl+c":
		return model, tea.Quit
	case "esc":
		model.writingGuidance = false
		model.guidanceInput.Blur()
		return model, nil
	case "enter":
		model.writingGuidance = false
		model.guidanceInput.Blur()
		model.startScreening(fmt.Sprintf("comment #%d", model.guidanceNumber), []int{model.guidanceNumber},
			"comment", strconv.Itoa(model.guidanceNumber), "--guidance", model.guidanceInput.Value())
		return model, nil
	}
	var command tea.Cmd
	model.guidanceInput, command = model.guidanceInput.Update(message)
	return model, command
}
