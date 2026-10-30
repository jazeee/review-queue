package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	query      string
}

type identityMsg struct {
	Identity Identity
	Err      error
}

type pollTickMsg struct{}

type piLaunchedMsg struct {
	Text string
	Err  error
}

type watchedRefreshedMsg struct {
	PullRequests []PullRequest
	Err          error
}
type clockTickMsg struct{}

type Model struct {
	repository string
	checkout   string
	window     time.Duration
	store      Store
	cache      Cache
	dismissals map[int]time.Time

	allRows []QueueRow
	rows    []QueueRow
	view    ViewKind
	cursor  int
	offset  int

	searching   bool
	searchInput textinput.Model

	width, height int
	mouseEnabled  bool

	events        chan tea.Msg
	scanning      bool
	scanCompleted int
	scanTotal     int
	lastSyncedAt  time.Time
	lastError     string
	flash         string
	confirmAll    bool
	undo          map[int]time.Time
	rescanQueued  bool
	filters       map[ViewKind]viewFilter

	lastClickNumber int
	lastClickAt     time.Time
	// watched holds PRs recently opened in the browser, keyed to when they were opened.
	watched map[int]time.Time
}

func NewModel(repository string, checkout string, window time.Duration) Model {
	store := NewStore(repository)
	searchInput := textinput.New()
	searchInput.Prompt = "/"
	model := Model{
		repository:   repository,
		checkout:     checkout,
		window:       window,
		store:        store,
		cache:        store.LoadCache(),
		dismissals:   store.LoadDismissals(),
		view:         ViewInbox,
		searchInput:  searchInput,
		mouseEnabled: true,
		events:       make(chan tea.Msg, 64),
		watched:      map[int]time.Time{},
		filters:      map[ViewKind]viewFilter{},
	}
	model.rebuildRows()
	return model
}

func (model Model) Init() tea.Cmd {
	organization := strings.SplitN(model.repository, "/", 2)[0]
	return tea.Batch(
		model.waitForEvent(),
		func() tea.Msg {
			identity, identityError := FetchIdentity(organization)
			return identityMsg{Identity: identity, Err: identityError}
		},
		tea.Tick(time.Second, func(time.Time) tea.Msg { return clockTickMsg{} }),
	)
}

func (model Model) waitForEvent() tea.Cmd {
	return func() tea.Msg { return <-model.events }
}

func (model *Model) startScan() {
	if model.scanning || model.cache.Identity.Login == "" {
		return
	}
	from := time.Now().Add(-model.window)
	if overlap := model.cache.ScannedThrough.Add(-5 * time.Minute); overlap.After(from) {
		from = overlap
	}
	model.scanning = true
	model.scanCompleted, model.scanTotal = 0, 0
	StartScan(model.repository, from, model.events)
}

func (model Model) refreshWatched() tea.Cmd {
	var numbers []int
	for number, openedAt := range model.watched {
		if time.Since(openedAt) > 30*time.Minute {
			delete(model.watched, number)
		} else {
			numbers = append(numbers, number)
		}
	}
	if len(numbers) == 0 {
		return nil
	}
	return func() tea.Msg {
		pullRequests, fetchError := FetchPullRequests(model.repository, numbers)
		return watchedRefreshedMsg{PullRequests: pullRequests, Err: fetchError}
	}
}

// openPullRequest also watches the PR so a review submitted in the browser shows up quickly.
func (model *Model) openPullRequest(pullRequest PullRequest) {
	model.watched[pullRequest.Number] = time.Now()
	if openError := OpenUrl(pullRequest.Url); openError != nil {
		model.lastError = openError.Error()
	}
}

func (model Model) isRelevant(pullRequest PullRequest) bool {
	if pullRequest.Author == model.cache.Identity.Login {
		return true
	}
	for _, event := range pullRequest.RequestEvents {
		if model.cache.Identity.owns(event.Reviewer) {
			return true
		}
	}
	return false
}

// mergePullRequests keeps the fresher copy, since a slow scan can land after a direct fetch.
func (model *Model) mergePullRequests(pullRequests []PullRequest) {
	for _, pullRequest := range pullRequests {
		cached, isCached := model.cache.PullRequests[pullRequest.Number]
		if isCached && cached.UpdatedAt.After(pullRequest.UpdatedAt) {
			continue
		}
		// GitHub reports UNKNOWN while it recomputes mergeability; keep the last answer for the same head.
		if isCached && pullRequest.Mergeable == "UNKNOWN" && cached.HeadOid == pullRequest.HeadOid {
			pullRequest.Mergeable = cached.Mergeable
		}
		if model.isRelevant(pullRequest) {
			model.cache.PullRequests[pullRequest.Number] = pullRequest
		}
	}
	model.rebuildRows()
}

func (model *Model) rebuildRows() {
	horizon := time.Now().Add(-model.window)
	model.allRows = nil
	for number, pullRequest := range model.cache.PullRequests {
		isMineOpen := pullRequest.Author == model.cache.Identity.Login && pullRequest.isOpen()
		if pullRequest.UpdatedAt.Before(horizon) && !isMineOpen {
			delete(model.cache.PullRequests, number)
			continue
		}
		if pullRequest.State == PullRequestClosed || (pullRequest.State == PullRequestMerged && time.Since(pullRequest.MergedAt) > 5*24*time.Hour) {
			continue
		}
		if row, ok := BuildQueueRow(pullRequest, model.cache.Identity, model.dismissals[number]); ok {
			model.allRows = append(model.allRows, row)
		}
	}
	sort.Slice(model.allRows, func(left, right int) bool {
		leftRow, rightRow := model.allRows[left], model.allRows[right]
		if !leftRow.LatestRequestAt.Equal(rightRow.LatestRequestAt) {
			return leftRow.LatestRequestAt.After(rightRow.LatestRequestAt)
		}
		if !leftRow.PullRequest.UpdatedAt.Equal(rightRow.PullRequest.UpdatedAt) {
			return leftRow.PullRequest.UpdatedAt.After(rightRow.PullRequest.UpdatedAt)
		}
		return leftRow.PullRequest.Number > rightRow.PullRequest.Number
	})
	model.applyFilter()
}

func belongsToView(row QueueRow, view ViewKind, draftsOnly bool) bool {
	isMine := row.MyStatus == MyStatusAuthor
	isDraft := row.PullRequest.State == PullRequestDraft
	if draftsOnly && !isDraft {
		return false
	}
	if !draftsOnly && isDraft && !isMine {
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
		if belongsToView(row, view, model.filters[view].draftsOnly) {
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
		if !belongsToView(row, model.view, filter.draftsOnly) {
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

func (model *Model) clampCursor() {
	model.cursor = max(0, min(model.cursor, len(model.rows)-1))
	visible := max(1, model.tableHeight())
	if model.cursor < model.offset {
		model.offset = model.cursor
	}
	if model.cursor >= model.offset+visible {
		model.offset = model.cursor - visible + 1
	}
	model.offset = max(0, min(model.offset, len(model.rows)-visible))
}

func (model Model) selectedRow() (QueueRow, bool) {
	if model.cursor < len(model.rows) {
		return model.rows[model.cursor], true
	}
	return QueueRow{}, false
}

func (model *Model) setDismissed(rows []QueueRow, dismissed bool) {
	model.undo = map[int]time.Time{}
	for _, row := range rows {
		model.undo[row.PullRequest.Number] = model.dismissals[row.PullRequest.Number]
		if dismissed {
			model.dismissals[row.PullRequest.Number] = time.Now()
		} else {
			delete(model.dismissals, row.PullRequest.Number)
		}
	}
	if saveError := model.store.SaveDismissals(model.dismissals); saveError != nil {
		model.lastError = saveError.Error()
	}
	model.rebuildRows()
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = message.Width, message.Height
		model.clampCursor()
		return model, nil

	case identityMsg:
		if message.Err != nil {
			model.lastError = message.Err.Error()
			if model.cache.Identity.Login == "" {
				return model, nil
			}
		} else {
			model.cache.Identity = message.Identity
			model.rebuildRows()
		}
		model.startScan()
		return model, nil

	case scanBatchMsg:
		model.scanCompleted, model.scanTotal = message.Completed, message.Total
		model.mergePullRequests(message.PullRequests)
		return model, model.waitForEvent()

	case scanDoneMsg:
		model.scanning = false
		if message.Err != nil {
			model.lastError = message.Err.Error()
		} else {
			model.lastError = ""
			model.cache.ScannedThrough = message.StartedAt
			model.lastSyncedAt = time.Now()
		}
		if saveError := model.store.SaveCache(model.cache); saveError != nil {
			model.lastError = saveError.Error()
		}
		if model.rescanQueued {
			model.rescanQueued = false
			model.cache.ScannedThrough = time.Time{}
			model.startScan()
		}
		return model, tea.Batch(
			model.waitForEvent(),
			tea.Tick(30*time.Second, func(time.Time) tea.Msg { return pollTickMsg{} }),
		)

	case pollTickMsg:
		model.startScan()
		return model, model.refreshWatched()

	case tea.FocusMsg:
		model.startScan()
		return model, model.refreshWatched()

	case piLaunchedMsg:
		if message.Err != nil {
			model.lastError = message.Err.Error()
		} else {
			model.flash = message.Text
		}
		return model, nil

	case watchedRefreshedMsg:
		if message.Err != nil {
			model.lastError = message.Err.Error()
			return model, nil
		}
		model.mergePullRequests(message.PullRequests)
		return model, nil

	case clockTickMsg:
		return model, tea.Tick(time.Second, func(time.Time) tea.Msg { return clockTickMsg{} })

	case tea.MouseMsg:
		switch {
		case message.Button == tea.MouseButtonWheelUp:
			model.cursor -= model.tableHeight()
			model.offset -= model.tableHeight()
		case message.Button == tea.MouseButtonWheelDown:
			model.cursor += model.tableHeight()
			model.offset += model.tableHeight()
		case message.Button == tea.MouseButtonLeft && message.Action == tea.MouseActionPress && message.Y == 0:
			tabStart := 0
			for index := range views {
				tabWidth := lipgloss.Width(styleTab.Render(model.formatTabLabel(index)))
				if message.X >= tabStart && message.X < tabStart+tabWidth {
					model.switchView(views[index])
				}
				tabStart += tabWidth + 1
			}
		case message.Button == tea.MouseButtonLeft && message.Action == tea.MouseActionPress:
			// Rows start below the tab bar and the column header.
			if row := message.Y - 2; row >= 0 && row < model.tableHeight() && model.offset+row < len(model.rows) {
				model.cursor = model.offset + row
				number := model.rows[model.cursor].PullRequest.Number
				if number == model.lastClickNumber && time.Since(model.lastClickAt) < 400*time.Millisecond {
					model.openPullRequest(model.rows[model.cursor].PullRequest)
					model.lastClickAt = time.Time{}
				} else {
					model.lastClickNumber, model.lastClickAt = number, time.Now()
				}
			}
		}
		model.clampCursor()
		return model, nil

	case tea.KeyMsg:
		if model.searching {
			return model.updateSearch(message)
		}
		return model.updateKey(message)
	}
	return model, nil
}

func (model Model) updateSearch(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		model.searching = false
		model.searchInput.Blur()
		model.setQuery("")
		return model, nil
	case "enter":
		model.searching = false
		model.searchInput.Blur()
		return model, nil
	}
	var command tea.Cmd
	model.searchInput, command = model.searchInput.Update(message)
	model.setQuery(model.searchInput.Value())
	return model, command
}

func (model Model) updateKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := message.String()
	confirmAll := model.confirmAll
	model.confirmAll = false
	model.flash = ""
	row, hasRow := model.selectedRow()

	switch key {
	case "q", "ctrl+c":
		return model, tea.Quit
	case "up", "k":
		model.cursor--
	case "down", "j":
		model.cursor++
	case "pgup":
		model.cursor -= model.tableHeight()
	case "pgdown":
		model.cursor += model.tableHeight()
	case "home", "g":
		model.cursor = 0
	case "end", "G":
		model.cursor = len(model.rows) - 1
	case "tab", "shift+tab", "1", "2", "3", "4":
		index := 0
		for candidate, view := range views {
			if view == model.view {
				index = candidate
			}
		}
		switch key {
		case "tab":
			index = (index + 1) % len(views)
		case "shift+tab":
			index = (index + len(views) - 1) % len(views)
		default:
			index = int(key[0] - '1')
		}
		model.switchView(views[index])
	case "/":
		model.searching = true
		model.searchInput.Focus()
		return model, textinput.Blink
	case "esc":
		model.setQuery("")
	case "o", "enter":
		if hasRow {
			model.openPullRequest(row.PullRequest)
		}
	case "p":
		if !hasRow {
			return model, nil
		}
		pullRequest := row.PullRequest
		model.flash = fmt.Sprintf("preparing a worktree for #%d…", pullRequest.Number)
		return model, func() tea.Msg {
			path, worktreeError := EnsurePullRequestWorktree(model.checkout, pullRequest)
			if worktreeError != nil {
				return piLaunchedMsg{Err: worktreeError}
			}
			terminalName, launchError := LaunchPiInTerminal(path)
			return piLaunchedMsg{Text: fmt.Sprintf("opened pi in %s: %s", terminalName, path), Err: launchError}
		}
	case "y":
		if hasRow {
			if copyError := CopyToSystemClipboard(row.PullRequest.Url); copyError != nil {
				model.lastError = copyError.Error()
			} else {
				model.flash = "copied " + row.PullRequest.Url
			}
		}
	case " ":
		if hasRow && row.MyStatus != MyStatusAuthor {
			model.setDismissed([]QueueRow{row}, !row.Dismissed)
		}
	case "A":
		if !confirmAll {
			model.confirmAll = true
			model.flash = fmt.Sprintf("press A again to mark %d rows seen", len(model.rows))
			return model, nil
		}
		model.setDismissed(model.rows, true)
	case "u":
		for number, dismissedAt := range model.undo {
			if dismissedAt.IsZero() {
				delete(model.dismissals, number)
			} else {
				model.dismissals[number] = dismissedAt
			}
		}
		model.flash = fmt.Sprintf("restored %d rows", len(model.undo))
		model.undo = nil
		if saveError := model.store.SaveDismissals(model.dismissals); saveError != nil {
			model.lastError = saveError.Error()
		}
		model.rebuildRows()
	case "d":
		filter := model.filters[model.view]
		filter.draftsOnly = !filter.draftsOnly
		model.filters[model.view] = filter
		model.cursor, model.offset = 0, 0
		model.applyFilter()
	case "r":
		model.startScan()
	case "R":
		if model.scanning {
			model.rescanQueued = true
			model.flash = "full rescan queued after the current scan"
		} else {
			model.cache.ScannedThrough = time.Time{}
			model.startScan()
		}
	case "m":
		model.mouseEnabled = !model.mouseEnabled
		if model.mouseEnabled {
			return model, tea.EnableMouseCellMotion
		}
		return model, tea.DisableMouse
	}
	model.clampCursor()
	return model, nil
}
