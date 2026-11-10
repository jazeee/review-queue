package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const detailHeight = 11

// renderDetailPane shows the selected PR on the left and rote-review's verdict on the
// right, each clipped to its column and to the pane's height.
func (model Model) renderDetailPane() string {
	leftWidth := model.width * 55 / 100
	rightWidth := max(1, model.width-leftWidth-3)
	left := model.renderDetail()
	right := []string{styleDim.Render("rote-review: not screened")}
	if row, hasRow := model.selectedRow(); !hasRow {
		right = nil
	} else if verdictLines := renderVerdictDetail(row.PullRequest, model.verdicts, rightWidth); len(verdictLines) > 0 {
		right = verdictLines
	}
	if row, hasRow := model.selectedRow(); hasRow && row.MyStatus == MyStatusAuthor && row.PullRequest.isOpen() {
		right = append(renderAutomationDetail(model.automations.Lookup(row.PullRequest.Number), rightWidth), right...)
	}
	lines := make([]string, detailHeight-1)
	for index := range lines {
		var leftLine, rightLine string
		if index < len(left) {
			leftLine = ansi.Truncate(left[index], leftWidth, "…")
		}
		if index < len(right) {
			rightLine = ansi.Truncate(right[index], rightWidth, "…")
		}
		lines[index] = leftLine + strings.Repeat(" ", max(0, leftWidth-ansi.StringWidth(leftLine))) + styleDim.Render(" │ ") + rightLine
	}
	return styleDetail.Width(model.width).Render(strings.Join(lines, "\n"))
}

func (model Model) renderDetail() []string {
	row, hasRow := model.selectedRow()
	if !hasRow {
		return []string{styleDim.Render("Nothing here.")}
	}
	pullRequest := row.PullRequest
	lines := []string{
		styleBold.Render(fmt.Sprintf("#%d %s", pullRequest.Number, pullRequest.Title)),
		fmt.Sprintf("%s · %s · %s %s · opened %s ago", pullRequest.Author,
			stateStyle(pullRequest.State).Render(string(pullRequest.State)),
			styleGood.Render(fmt.Sprintf("+%d", pullRequest.Additions)), styleBad.Render(fmt.Sprintf("-%d", pullRequest.Deletions)),
			formatAge(pullRequest.CreatedAt)),
		pullRequest.Url,
	}
	if pullRequest.BaseRef != "main" {
		lines = append(lines, styleWarning.Render(" stacked on "+pullRequest.BaseRef+" "))
	}
	if pullRequest.isOpen() && pullRequest.Mergeable == "CONFLICTING" {
		lines = append(lines, styleAlert.Render(" merge conflicts with "+pullRequest.BaseRef+" "))
	}
	for _, target := range row.Targets {
		name := "@" + target.Reviewer.Name
		if target.Reviewer.Kind == ReviewerTeam {
			name = "team " + target.Reviewer.Name
		}
		status := stylePending.Render("pending")
		switch target.Status {
		case TargetFulfilled:
			status = styleGood.Render("reviewed by " + target.ResolvedBy)
		case TargetRemoved:
			status = styleDim.Render("removed")
			if target.ResolvedBy != "" {
				status = styleDim.Render("removed by " + target.ResolvedBy)
			}
		}
		lines = append(lines, fmt.Sprintf("  %-28s %s  %s", name, status,
			styleDim.Render(fmt.Sprintf("requested %s ago by %s", formatAge(target.RequestedAt), target.RequestedBy))))
	}
	var reviews []string
	for index := len(pullRequest.Reviews) - 1; index >= 0 && len(reviews) < 6; index-- {
		review := pullRequest.Reviews[index]
		if isAutomatedReviewer(review.Author) {
			continue
		}
		style := styleDim
		switch review.State {
		case "APPROVED":
			style = styleGood
		case "CHANGES_REQUESTED":
			style = styleBad
		}
		reviews = append(reviews, fmt.Sprintf("%s %s %s", review.Author,
			style.Render(strings.ToLower(strings.ReplaceAll(review.State, "_", " "))), styleDim.Render(formatAge(review.SubmittedAt))))
	}
	if len(reviews) > 0 {
		lines = append(lines, "Reviews: "+strings.Join(reviews, " · "))
	}
	return lines
}
