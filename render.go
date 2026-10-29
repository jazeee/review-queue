package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const detailHeight = 9

type segment struct {
	text  string
	style lipgloss.Style
}

func (model Model) tableHeight() int {
	return model.height - 3 - detailHeight
}

func formatAge(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	elapsed := time.Since(at)
	switch {
	case elapsed < time.Minute:
		return fmt.Sprintf("%ds", int(elapsed.Seconds()))
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	case elapsed < 48*time.Hour:
		return fmt.Sprintf("%dh", int(elapsed.Hours()))
	default:
		return fmt.Sprintf("%dd", int(elapsed.Hours()/24))
	}
}

// renderCell truncates plain text before styling so escape sequences are never cut.
func renderCell(segments []segment, width int, selected bool) string {
	var builder strings.Builder
	remaining := width
	paint := func(style lipgloss.Style, text string) {
		if _, unset := style.GetBackground().(lipgloss.NoColor); selected && unset {
			style = style.Background(colorHighlight)
		}
		builder.WriteString(style.Render(text))
	}
	for _, part := range segments {
		if remaining <= 0 {
			break
		}
		text := part.text
		if ansi.StringWidth(text) > remaining {
			text = ansi.Truncate(text, remaining, "…")
		}
		paint(part.style, text)
		remaining -= ansi.StringWidth(text)
	}
	if remaining > 0 {
		paint(lipgloss.NewStyle(), strings.Repeat(" ", remaining))
	}
	return builder.String()
}

func renderTargets(row QueueRow) []segment {
	var segments []segment
	if row.MyStatus == MyStatusAuthor {
		for index, reviewer := range row.PullRequest.PendingReviewers {
			if index > 0 {
				segments = append(segments, segment{",", styleDim})
			}
			segments = append(segments, segment{reviewer.Name, stylePending})
		}
		return segments
	}
	for index, target := range row.Targets {
		if index > 0 {
			segments = append(segments, segment{",", styleDim})
		}
		name := target.Reviewer.Name
		if target.Reviewer.Kind == ReviewerUser {
			name = "@me"
		}
		style := styleStruck
		if target.Status == TargetPending {
			style = stylePending
		}
		segments = append(segments, segment{name, style})
	}
	return segments
}

func renderState(pullRequest PullRequest) []segment {
	segments := []segment{{string(pullRequest.State), stateStyle(pullRequest.State)}}
	switch pullRequest.ReviewDecision {
	case "APPROVED":
		segments = append(segments, segment{" ✓", styleGood})
	case "CHANGES_REQUESTED":
		segments = append(segments, segment{" ✗", styleBad})
	}
	return segments
}

func renderMyStatus(row QueueRow) segment {
	switch {
	case row.MyStatus == MyStatusAuthor:
		return segment{"mine", styleDim}
	case row.MyStatus == MyStatusReviewed:
		return segment{"reviewed", styleGood}
	case row.MyStatus == MyStatusUpdated:
		return segment{"+commits", stylePending}
	case row.Dismissed:
		return segment{"seen", styleDim}
	default:
		return segment{string(row.MyStatus), styleUnread}
	}
}

type column struct {
	title string
	width int
}

func (model Model) columns() []column {
	columns := []column{{"", 1}, {"PR", 6}, {"Age", 4}, {"Author", 16}, {"State", 8}, {"Lines", 6}, {"+", 5}, {"-", 5}, {"For", 26}, {"Me", 8}}
	used := 0
	for _, entry := range columns {
		used += entry.width + 1
	}
	return append(columns, column{"Title", max(10, model.width-used)})
}

func (model Model) renderRow(row QueueRow, selected bool) string {
	pullRequest := row.PullRequest
	dot := segment{" ", lipgloss.NewStyle()}
	if row.Unread {
		dot = segment{"●", styleUnread}
	}
	titleStyle := lipgloss.NewStyle()
	switch {
	case row.MyStatus != MyStatusAuthor && row.Settled():
		titleStyle = styleStruck
	case row.Unread:
		titleStyle = styleBold
	case !pullRequest.isOpen():
		titleStyle = styleDim
	}
	age := row.LatestRequestAt
	if row.MyStatus == MyStatusAuthor {
		age = pullRequest.UpdatedAt
	}
	cells := [][]segment{
		{dot},
		{{fmt.Sprintf("%d", pullRequest.Number), styleDim}},
		{{formatAge(age), styleDim}},
		{{pullRequest.Author, lipgloss.NewStyle()}},
		renderState(pullRequest),
		{{fmt.Sprintf("%5d ", pullRequest.Additions+pullRequest.Deletions), sizeStyle(pullRequest.Additions + pullRequest.Deletions)}},
		{{fmt.Sprintf("%5d", pullRequest.Additions), styleGood}},
		{{fmt.Sprintf("%5d", pullRequest.Deletions), styleBad}},
		renderTargets(row),
		{renderMyStatus(row)},
		{{pullRequest.Title, titleStyle}},
	}
	rendered := make([]string, len(cells))
	for index, entry := range model.columns() {
		rendered[index] = renderCell(cells[index], entry.width, selected)
	}
	return strings.Join(rendered, renderCell(nil, 1, selected))
}

func (model Model) renderDetail() []string {
	row, hasRow := model.selectedRow()
	if !hasRow {
		return []string{styleDim.Render("Nothing here.")}
	}
	pullRequest := row.PullRequest
	lines := []string{
		styleBold.Render(fmt.Sprintf("#%d %s", pullRequest.Number, pullRequest.Title)),
		fmt.Sprintf("%s · %s · %s %s · opened %s ago · %s", pullRequest.Author,
			stateStyle(pullRequest.State).Render(string(pullRequest.State)),
			styleGood.Render(fmt.Sprintf("+%d", pullRequest.Additions)), styleBad.Render(fmt.Sprintf("-%d", pullRequest.Deletions)),
			formatAge(pullRequest.CreatedAt), pullRequest.Url),
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

// formatTabLabel is shared with mouse hit-testing so click targets match what is drawn.
func (model Model) formatTabLabel(index int) string {
	return fmt.Sprintf("%d %s %d", index+1, views[index], model.countView(views[index]))
}

func (model Model) renderHeader() string {
	var tabs []string
	for index, view := range views {
		label := model.formatTabLabel(index)
		if view == model.view {
			tabs = append(tabs, styleTabFocus.Render(label))
		} else {
			tabs = append(tabs, styleTab.Render(label))
		}
	}
	left := strings.Join(tabs, " ")
	if model.draftsOnly {
		left += "  " + stylePending.Render("drafts only")
	}
	var status string
	switch {
	case model.scanning && model.scanTotal > 0:
		status = stylePending.Render(fmt.Sprintf("scanning %d/%d", model.scanCompleted, model.scanTotal))
	case model.scanning:
		status = stylePending.Render("scanning…")
	case !model.lastSyncedAt.IsZero():
		status = styleDim.Render("synced " + formatAge(model.lastSyncedAt) + " ago")
	}
	if model.lastError != "" {
		status = styleError.Render(ansi.Truncate(model.lastError, 60, "…")) + " " + status
	}
	gap := max(1, model.width-lipgloss.Width(left)-lipgloss.Width(status))
	return left + strings.Repeat(" ", gap) + status
}

func (model Model) View() string {
	if model.width == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, model.renderHeader())

	var header []string
	for _, entry := range model.columns() {
		header = append(header, renderCell([]segment{{entry.title, styleHeading}}, entry.width, false))
	}
	lines = append(lines, strings.Join(header, " "))

	tableHeight := model.tableHeight()
	for index := model.offset; index < model.offset+tableHeight; index++ {
		if index < len(model.rows) {
			lines = append(lines, model.renderRow(model.rows[index], index == model.cursor))
		} else if index == 0 {
			lines = append(lines, styleDim.Render("  Inbox zero."))
		} else {
			lines = append(lines, "")
		}
	}

	detail := model.renderDetail()
	for len(detail) < detailHeight-1 {
		detail = append(detail, "")
	}
	for index := range detail {
		detail[index] = ansi.Truncate(detail[index], model.width, "…")
	}
	lines = append(lines, styleDetail.Width(model.width).Render(strings.Join(detail[:detailHeight-1], "\n")))

	footer := styleDim.Render("o open · space seen · A all seen · u undo · / search · tab view · d drafts · r refresh · y copy · m mouse · q quit")
	switch {
	case model.searching:
		footer = model.searchInput.View()
	case model.flash != "":
		footer = stylePending.Render(model.flash)
	case model.searchPattern != nil:
		footer = styleDim.Render("filter: ") + model.searchInput.Value() + styleDim.Render("  (esc clears)")
	}
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}
