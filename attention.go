package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Verdict is the part of a rote-review verdict that review-queue shows.
type Verdict struct {
	Number      int       `json:"number"`
	HeadOid     string    `json:"headOid"`
	Attention   string    `json:"attention"`
	Summary     string    `json:"summary"`
	Held        []string  `json:"held"`
	EvaluatedAt time.Time `json:"evaluatedAt"`
}

// Verdicts maps "number:headOid" to the latest rote-review verdict for that head.
type Verdicts struct {
	path       string
	modifiedAt time.Time
	byHead     map[string]Verdict
}

// NewVerdicts reads the file rote-review writes for the same repository.
func NewVerdicts(repository string) Verdicts {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}
	path := filepath.Join(stateHome, "rote-review", strings.ReplaceAll(repository, "/", "-"), "verdicts.json")
	return Verdicts{path: path, byHead: map[string]Verdict{}}
}

// Reload rereads the file only when rote-review has rewritten it.
func (verdicts *Verdicts) Reload() {
	info, statError := os.Stat(verdicts.path)
	if statError != nil || info.ModTime().Equal(verdicts.modifiedAt) {
		return
	}
	contents, readError := os.ReadFile(verdicts.path)
	var stored map[string]Verdict
	if readError != nil || json.Unmarshal(contents, &stored) != nil {
		return
	}
	verdicts.modifiedAt = info.ModTime()
	verdicts.byHead = map[string]Verdict{}
	for _, verdict := range stored {
		key := fmt.Sprintf("%d:%s", verdict.Number, verdict.HeadOid)
		if verdict.Attention != "" && verdict.EvaluatedAt.After(verdicts.byHead[key].EvaluatedAt) {
			verdicts.byHead[key] = verdict
		}
	}
}

// Lookup only answers for the pull request's current head; an older verdict is stale.
func (verdicts Verdicts) Lookup(pullRequest PullRequest) (Verdict, bool) {
	verdict, found := verdicts.byHead[fmt.Sprintf("%d:%s", pullRequest.Number, pullRequest.HeadOid)]
	return verdict, found
}

func attentionStyle(attention string) lipgloss.Style {
	badge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0"))
	switch attention {
	case "rote":
		return badge.Background(lipgloss.Color("#00af00"))
	case "light":
		return badge.Background(lipgloss.Color("#afd700"))
	case "normal":
		return badge.Background(lipgloss.Color("#ffd700"))
	default:
		return badge.Foreground(lipgloss.Color("15")).Background(lipgloss.Color("#d70000"))
	}
}

var styleHeld = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("#ff8700"))

func renderNotes(pullRequest PullRequest, verdicts Verdicts) []segment {
	if pullRequest.isOpen() && pullRequest.Mergeable == "CONFLICTING" {
		return []segment{{"conflicts", styleAlert}}
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
