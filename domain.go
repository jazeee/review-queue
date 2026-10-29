package main

import "time"

type PullRequestState string

const (
	PullRequestOpen   PullRequestState = "open"
	PullRequestDraft  PullRequestState = "draft"
	PullRequestMerged PullRequestState = "merged"
	PullRequestClosed PullRequestState = "closed"
)

type ReviewerKind string

const (
	ReviewerUser ReviewerKind = "user"
	ReviewerTeam ReviewerKind = "team"
)

type Reviewer struct {
	Kind ReviewerKind `json:"kind"`
	Name string       `json:"name"`
}

type RequestAction string

const (
	RequestAdded   RequestAction = "requested"
	RequestRemoved RequestAction = "removed"
)

type RequestEvent struct {
	Reviewer Reviewer      `json:"reviewer"`
	Action   RequestAction `json:"action"`
	At       time.Time     `json:"at"`
	Actor    string        `json:"actor"`
}

type Review struct {
	Author      string    `json:"author"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submittedAt"`
	CommitOid   string    `json:"commitOid"`
}

type PullRequest struct {
	Number           int              `json:"number"`
	Title            string           `json:"title"`
	Url              string           `json:"url"`
	Author           string           `json:"author"`
	State            PullRequestState `json:"state"`
	ReviewDecision   string           `json:"reviewDecision"`
	CreatedAt        time.Time        `json:"createdAt"`
	UpdatedAt        time.Time        `json:"updatedAt"`
	MergedAt         time.Time        `json:"mergedAt"`
	HeadOid          string           `json:"headOid"`
	PendingReviewers []Reviewer       `json:"pendingReviewers"`
	RequestEvents    []RequestEvent   `json:"requestEvents"`
	Reviews          []Review         `json:"reviews"`
}

type Identity struct {
	Login string   `json:"login"`
	Teams []string `json:"teams"`
}

func (identity Identity) owns(reviewer Reviewer) bool {
	if reviewer.Kind == ReviewerUser {
		return reviewer.Name == identity.Login
	}
	for _, team := range identity.Teams {
		if team == reviewer.Name {
			return true
		}
	}
	return false
}

func (pullRequest PullRequest) isOpen() bool {
	return pullRequest.State == PullRequestOpen || pullRequest.State == PullRequestDraft
}
