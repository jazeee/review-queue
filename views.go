package main

import (
	"fmt"
	"regexp"
	"strings"
)

type ViewKind string

const (
	ViewInbox   ViewKind = "Inbox"
	ViewWaiting ViewKind = "Waiting"
	ViewAll     ViewKind = "All"
	ViewMine    ViewKind = "Mine"
)

var views = []ViewKind{ViewInbox, ViewWaiting, ViewAll, ViewMine}

type viewFilter struct {
	draftsOnly bool
	// hideStacked hides PRs whose base is not main; Inbox starts with it on.
	hideStacked bool
	query       string
}

func belongsToView(row QueueRow, view ViewKind, filter viewFilter) bool {
	isMine := row.MyStatus == MyStatusAuthor
	isDraft := row.PullRequest.State == PullRequestDraft
	if filter.draftsOnly && !isDraft {
		return false
	}
	if !filter.draftsOnly && isDraft && !isMine {
		return false
	}
	if filter.hideStacked && row.PullRequest.BaseRef != "main" {
		return false
	}
	switch view {
	case ViewInbox:
		return row.Unread
	case ViewWaiting:
		return !isMine && row.PullRequest.isOpen() && !row.Settled() && row.MyStatus != MyStatusReviewed
	case ViewAll:
		return !isMine
	default:
		return isMine && row.PullRequest.State != PullRequestMerged && !row.PullRequest.hasLabel("WIP")
	}
}

func (model Model) countView(view ViewKind) int {
	count := 0
	for _, row := range model.allRows {
		if belongsToView(row, view, model.filters[view]) {
			count++
		}
	}
	return count
}

func (model *Model) applyFilter() {
	selectedNumber := 0
	if model.cursor < len(model.rows) {
		selectedNumber = model.rows[model.cursor].PullRequest.Number
	}
	model.rows = nil
	filter := model.filters[model.view]
	var pattern *regexp.Regexp
	if filter.query != "" {
		var compileError error
		if pattern, compileError = regexp.Compile("(?i)" + filter.query); compileError != nil {
			pattern = regexp.MustCompile("(?i)" + regexp.QuoteMeta(filter.query))
		}
	}
	for _, row := range model.allRows {
		if !belongsToView(row, model.view, filter) {
			continue
		}
		if pattern != nil && !pattern.MatchString(searchableText(row)) {
			continue
		}
		model.rows = append(model.rows, row)
	}
	for index, row := range model.rows {
		if row.PullRequest.Number == selectedNumber {
			model.cursor = index
			model.clampCursor()
			return
		}
	}
	model.clampCursor()
}

func (model *Model) switchView(view ViewKind) {
	model.view = view
	model.cursor, model.offset = 0, 0
	model.searchInput.SetValue(model.filters[view].query)
	model.applyFilter()
}

func (model *Model) setQuery(query string) {
	filter := model.filters[model.view]
	filter.query = query
	model.filters[model.view] = filter
	model.searchInput.SetValue(query)
	model.applyFilter()
}

func searchableText(row QueueRow) string {
	parts := []string{fmt.Sprintf("#%d", row.PullRequest.Number), row.PullRequest.Title, row.PullRequest.Author}
	for _, target := range row.Targets {
		parts = append(parts, target.Reviewer.Name, target.ResolvedBy)
	}
	return strings.Join(parts, " ")
}

// countHiddenStacked counts the current tab's stacked PRs that hideStacked keeps out of view.
func (model Model) countHiddenStacked() int {
	filter := model.filters[model.view]
	if !filter.hideStacked {
		return 0
	}
	shown := filter
	shown.hideStacked = false
	count := 0
	for _, row := range model.allRows {
		if row.PullRequest.BaseRef != "main" && belongsToView(row, model.view, shown) {
			count++
		}
	}
	return count
}
