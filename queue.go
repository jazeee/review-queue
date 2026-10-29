package main

import (
	"sort"
	"strings"
	"time"
)

type TargetStatus string

const (
	TargetPending   TargetStatus = "pending"
	TargetFulfilled TargetStatus = "fulfilled"
	TargetRemoved   TargetStatus = "removed"
)

// Target is one review request aimed at the viewer or one of the viewer's teams.
type Target struct {
	Reviewer    Reviewer
	RequestedAt time.Time
	RequestedBy string
	Status      TargetStatus
	ResolvedBy  string
}

type MyStatus string

const (
	MyStatusUnreviewed  MyStatus = "new"
	MyStatusRerequested MyStatus = "re-req"
	MyStatusReviewed    MyStatus = "reviewed"
	MyStatusUpdated     MyStatus = "+commits"
	MyStatusAuthor      MyStatus = "mine"
)

type QueueRow struct {
	PullRequest     PullRequest
	Targets         []Target
	MyStatus        MyStatus
	LatestRequestAt time.Time
	Dismissed       bool
	Unread          bool
}

// Settled means no request aimed at the viewer is still pending: Slack's strikethrough.
func (row QueueRow) Settled() bool {
	for _, target := range row.Targets {
		if target.Status == TargetPending {
			return false
		}
	}
	return true
}

func isAutomatedReviewer(login string) bool {
	return strings.HasSuffix(login, "[bot]") || strings.Contains(login, "copilot")
}

func buildTargets(pullRequest PullRequest, identity Identity) []Target {
	latestByReviewer := map[Reviewer]RequestEvent{}
	for _, event := range pullRequest.RequestEvents {
		if event.Action == RequestAdded && identity.owns(event.Reviewer) && event.At.After(latestByReviewer[event.Reviewer].At) {
			latestByReviewer[event.Reviewer] = event
		}
	}
	pending := map[Reviewer]bool{}
	for _, reviewer := range pullRequest.PendingReviewers {
		pending[reviewer] = true
	}

	targets := make([]Target, 0, len(latestByReviewer))
	for reviewer, request := range latestByReviewer {
		target := Target{Reviewer: reviewer, RequestedAt: request.At, RequestedBy: request.Actor, Status: TargetRemoved}
		if pending[reviewer] {
			target.Status = TargetPending
			targets = append(targets, target)
			continue
		}
		for _, event := range pullRequest.RequestEvents {
			if event.Action == RequestRemoved && event.Reviewer == reviewer && !event.At.Before(request.At) {
				target.ResolvedBy = event.Actor
			}
		}
		if target.ResolvedBy == "" {
			for _, review := range pullRequest.Reviews {
				if review.SubmittedAt.After(request.At) && review.Author != pullRequest.Author && !isAutomatedReviewer(review.Author) {
					target.Status = TargetFulfilled
					target.ResolvedBy = review.Author
					break
				}
			}
		}
		targets = append(targets, target)
	}
	sort.Slice(targets, func(left, right int) bool {
		if targets[left].Reviewer.Kind != targets[right].Reviewer.Kind {
			return targets[left].Reviewer.Kind == ReviewerUser
		}
		return targets[left].Reviewer.Name < targets[right].Reviewer.Name
	})
	return targets
}

// BuildQueueRow returns false when the pull request never asked the viewer, or the
// viewer's teams, for review and the viewer did not author it.
func BuildQueueRow(pullRequest PullRequest, identity Identity, dismissedAt time.Time) (QueueRow, bool) {
	row := QueueRow{PullRequest: pullRequest, Targets: buildTargets(pullRequest, identity)}
	if pullRequest.Author == identity.Login {
		row.MyStatus = MyStatusAuthor
		return row, true
	}
	if len(row.Targets) == 0 {
		return row, false
	}
	for _, target := range row.Targets {
		if target.RequestedAt.After(row.LatestRequestAt) {
			row.LatestRequestAt = target.RequestedAt
		}
	}

	var latestMine, latestMineBeforeRequest *Review
	for index := range pullRequest.Reviews {
		review := &pullRequest.Reviews[index]
		if review.Author != identity.Login {
			continue
		}
		if review.SubmittedAt.After(row.LatestRequestAt) {
			latestMine = review
		} else {
			latestMineBeforeRequest = review
		}
	}
	switch {
	case latestMine != nil && latestMine.CommitOid == pullRequest.HeadOid:
		row.MyStatus = MyStatusReviewed
	case latestMine != nil:
		row.MyStatus = MyStatusUpdated
	case latestMineBeforeRequest != nil:
		row.MyStatus = MyStatusRerequested
	default:
		row.MyStatus = MyStatusUnreviewed
	}

	row.Dismissed = !dismissedAt.Before(row.LatestRequestAt)
	row.Unread = !row.Dismissed && (row.MyStatus == MyStatusUnreviewed || row.MyStatus == MyStatusRerequested)
	return row, true
}
