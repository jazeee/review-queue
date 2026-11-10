package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

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
	columns := []column{{"", 1}, {"PR", 6}, {"Age", 4}, {"Author", 16}, {"State", 8}, {"+", 6}, {"-", 5}, {"For", 26}, {"Me", 8}, {"Review", 9}, {"Notes", 9}}
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
		{{fmt.Sprintf("%5d ", pullRequest.Additions), sizeStyle(pullRequest.Additions)}},
		{{fmt.Sprintf("%5d", pullRequest.Deletions), styleBad}},
		renderTargets(row),
		{renderMyStatus(row)},
		renderReview(pullRequest, model.verdicts, model.screening),
		renderNotes(pullRequest),
		{{pullRequest.Title, titleStyle}},
	}
	if pullRequest.BaseRef != "main" {
		cells[len(cells)-1] = []segment{{"⤷ " + pullRequest.BaseRef, styleWarning}, {" ", lipgloss.NewStyle()}, {pullRequest.Title, titleStyle}}
	}
	rendered := make([]string, len(cells))
	for index, entry := range model.columns() {
		rendered[index] = renderCell(cells[index], entry.width, selected)
	}
	return strings.Join(rendered, renderCell(nil, 1, selected))
}

// formatTabLabel is shared with mouse hit-testing so click targets match what is drawn.
func (model Model) formatTabLabel(index int) string {
	label := fmt.Sprintf("%d %s %d", index+1, views[index], model.countView(views[index]))
	if model.filters[views[index]].draftsOnly {
		label += " ✎"
	}
	if model.filters[views[index]].query != "" {
		label += " /"
	}
	return label
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
	if model.filters[model.view].draftsOnly {
		left += "  " + stylePending.Render("drafts only")
	}
	if hidden := model.countHiddenStacked(); hidden > 0 {
		left += "  " + stylePending.Render(fmt.Sprintf("%d stacked hidden (n)", hidden))
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
	if screeningStatus := model.renderScreeningStatus(); screeningStatus != "" {
		status = screeningStatus + "  " + status
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
	if model.dialog != nil {
		return model.renderVerdictDialog()
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

	lines = append(lines, model.renderDetailPane())

	footer := styleDim.Render("o open · p pi · s screen · S screen all · v view review · c post feedback · n stacked · space seen · A all seen · u undo · / search · tab view · d drafts · r refresh · y copy · m mouse · q quit")
	switch {
	case model.writingGuidance:
		footer = model.guidanceInput.View()
	case model.searching:
		footer = model.searchInput.View()
	case model.flash != "":
		footer = stylePending.Render(model.flash)
	case model.searchInput.Value() != "":
		footer = styleDim.Render("filter: ") + model.searchInput.Value() + styleDim.Render("  (esc clears)")
	}
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}
