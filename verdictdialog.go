package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// openVerdictDialog shows the selected PR's full rote-review verdict, scrollable.
func (model *Model) openVerdictDialog() {
	row, hasRow := model.selectedRow()
	if !hasRow {
		return
	}
	dialog := viewport.New(model.width, max(1, model.height-2))
	model.dialog = &dialog
	model.dialogNumber = row.PullRequest.Number
	model.refreshVerdictDialog()
}

func (model *Model) refreshVerdictDialog() {
	for _, row := range model.allRows {
		if row.PullRequest.Number == model.dialogNumber {
			model.dialog.Width, model.dialog.Height = model.width, max(1, model.height-2)
			model.dialog.SetContent(renderVerdictDocument(row.PullRequest, model.verdicts, model.width))
			return
		}
	}
}

// updateVerdictDialog handles input while the dialog is open; other messages (ticks,
// scan events) fall through to the normal update.
func (model *Model) updateVerdictDialog(message tea.Msg) (bool, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = message.Width, message.Height
		model.clampCursor()
		model.refreshVerdictDialog()
		return true, nil
	case tea.MouseMsg:
		updated, command := model.dialog.Update(message)
		*model.dialog = updated
		return true, command
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c":
			return true, tea.Quit
		case "esc", "q", "v":
			model.dialog = nil
		case "o":
			for _, row := range model.allRows {
				if row.PullRequest.Number == model.dialogNumber {
					model.openPullRequest(row.PullRequest)
				}
			}
		case "g", "home":
			model.dialog.GotoTop()
		case "G", "end":
			model.dialog.GotoBottom()
		default:
			updated, command := model.dialog.Update(message)
			*model.dialog = updated
			return true, command
		}
		return true, nil
	}
	return false, nil
}

func (model Model) renderVerdictDialog() string {
	header := styleTabFocus.Render(fmt.Sprintf("rote-review #%d", model.dialogNumber)) +
		styleDim.Render("  j/k pgup/pgdn g/G scroll · o open PR · esc close")
	footer := styleDim.Render(fmt.Sprintf("%3.0f%%", model.dialog.ScrollPercent()*100))
	return header + "\n" + model.dialog.View() + "\n" + footer
}

// renderVerdictDocument lays out everything rote-review knows about the PR's current head,
// word-wrapped to width with list items indented under their markers.
func renderVerdictDocument(pullRequest PullRequest, verdicts Verdicts, width int) string {
	var document strings.Builder
	write := func(text string, indent string) {
		for index, line := range strings.Split(ansi.Wrap(text, max(10, width-len(indent)-2), ""), "\n") {
			if index > 0 {
				document.WriteString(indent)
			}
			document.WriteString(line + "\n")
		}
	}
	section := func(title string) {
		document.WriteString("\n" + styleHeading.Render(title) + "\n")
	}

	write(styleBold.Render(fmt.Sprintf("#%d %s", pullRequest.Number, pullRequest.Title)), "")
	write(styleDim.Render(pullRequest.Url), "")
	verdict, found := verdicts.Lookup(pullRequest)
	if !found {
		document.WriteString("\nNot screened at this head. Press s to screen it.\n")
		return document.String()
	}
	facts := []string{verdict.Kind, verdict.Decision, "screened " + formatAge(verdict.EvaluatedAt) + " ago"}
	if verdict.ScreenedLines > 0 {
		facts = append(facts, fmt.Sprintf("%d lines read", verdict.ScreenedLines))
	}
	if verdict.SkippedFiles > 0 {
		facts = append(facts, fmt.Sprintf("%d test or lockfile sections skipped", verdict.SkippedFiles))
	}
	write(attentionStyle(verdict.Attention).Render(verdict.Attention)+" "+styleDim.Render(strings.Join(facts, " · ")), "")

	section("Summary")
	write("  "+verdict.Summary, "  ")
	if len(verdict.Violations) > 0 {
		section("Standards violations")
		for _, violation := range verdict.Violations {
			write("  "+styleBad.Render("✖ "+violation.Rule)+" "+violation.Path, "    ")
			write("    "+violation.Detail, "    ")
		}
	}
	if len(verdict.Held) > 0 {
		section("Held: reviewer feedback waiting on the author")
		for _, item := range verdict.Held {
			write("  - "+strings.TrimPrefix(item, "unanswered "), "    ")
		}
	}
	if len(verdict.Standards) > 0 {
		section("Standards notes")
		for _, note := range verdict.Standards {
			write("  "+stylePending.Render("◦")+" "+note, "    ")
		}
	}
	if len(verdict.Reasons) > 0 {
		section("Concerns")
		for _, reason := range verdict.Reasons {
			write("  - "+reason, "    ")
		}
	}
	if len(verdict.Chunks) > 1 {
		section("Chunks")
		for index, chunk := range verdict.Chunks {
			write(fmt.Sprintf("  %d. %s · %s confidence · %s risk · %s: %s", index+1, chunk.Verdict, chunk.Confidence, chunk.Risk, chunk.Category, chunk.Summary), "     ")
		}
	}
	return document.String()
}
