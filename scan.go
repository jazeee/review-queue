package main

import (
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type scanBatchMsg struct {
	PullRequests []PullRequest
	Completed    int
	Total        int
}

type scanDoneMsg struct {
	StartedAt time.Time
	Err       error
}

func scanWindow(repository string, from time.Time, to time.Time) ([]PullRequest, error) {
	query := fmt.Sprintf("repo:%s is:pr updated:%s..%s", repository, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	pullRequests, issueCount, searchError := SearchPullRequests(query)
	if searchError != nil || issueCount <= 1000 || to.Sub(from) < time.Minute {
		return pullRequests, searchError
	}
	middle := from.Add(to.Sub(from) / 2)
	earlier, earlierError := scanWindow(repository, from, middle)
	if earlierError != nil {
		return earlier, earlierError
	}
	later, laterError := scanWindow(repository, middle, to)
	return append(earlier, later...), laterError
}

// StartScan fetches every pull request updated in [from, now] in day-sized windows,
// plus the viewer's open pull requests, streaming results onto events.
func StartScan(repository string, from time.Time, events chan<- tea.Msg) {
	startedAt := time.Now()
	jobs := []func() ([]PullRequest, error){
		func() ([]PullRequest, error) {
			pullRequests, _, searchError := SearchPullRequests(fmt.Sprintf("repo:%s is:pr is:open author:@me", repository))
			return pullRequests, searchError
		},
	}
	for windowEnd := startedAt; windowEnd.After(from); windowEnd = windowEnd.Add(-24 * time.Hour) {
		windowStart := windowEnd.Add(-24 * time.Hour)
		if windowStart.Before(from) {
			windowStart = from
		}
		jobs = append(jobs, func() ([]PullRequest, error) { return scanWindow(repository, windowStart, windowEnd) })
	}

	go func() {
		queue := make(chan func() ([]PullRequest, error), len(jobs))
		for _, job := range jobs {
			queue <- job
		}
		close(queue)

		var mutex sync.Mutex
		var firstError error
		completed := 0
		var workers sync.WaitGroup
		for range 4 {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for job := range queue {
					pullRequests, jobError := job()
					mutex.Lock()
					completed++
					if jobError != nil && firstError == nil {
						firstError = jobError
					}
					message := scanBatchMsg{PullRequests: pullRequests, Completed: completed, Total: len(jobs)}
					mutex.Unlock()
					events <- message
				}
			}()
		}
		workers.Wait()
		events <- scanDoneMsg{StartedAt: startedAt, Err: firstError}
	}()
}
