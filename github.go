package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const pullRequestFields = `fragment PullRequestFields on PullRequest {
  number title url state isDraft reviewDecision createdAt updatedAt mergedAt baseRefName headRefOid
  author { login }
  reviewRequests(first: 30) { nodes { requestedReviewer { __typename ... on User { login } ... on Team { slug } } } }
  timelineItems(last: 100, itemTypes: [REVIEW_REQUESTED_EVENT, REVIEW_REQUEST_REMOVED_EVENT]) { nodes { __typename
    ... on ReviewRequestedEvent { createdAt actor { login } requestedReviewer { __typename ... on User { login } ... on Team { slug } } }
    ... on ReviewRequestRemovedEvent { createdAt actor { login } requestedReviewer { __typename ... on User { login } ... on Team { slug } } } } }
  reviews(last: 100) { nodes { author { login } state submittedAt commit { oid } } }
}`

const searchQuery = `query($q: String!, $after: String) {
  search(type: ISSUE, query: $q, first: 50, after: $after) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes { ...PullRequestFields }
  }
}
` + pullRequestFields

type actorPayload struct {
	Login string `json:"login"`
}

type reviewerPayload struct {
	Typename string `json:"__typename"`
	Login    string `json:"login"`
	Slug     string `json:"slug"`
}

type pullRequestPayload struct {
	Number         int           `json:"number"`
	Title          string        `json:"title"`
	Url            string        `json:"url"`
	State          string        `json:"state"`
	IsDraft        bool          `json:"isDraft"`
	ReviewDecision string        `json:"reviewDecision"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
	MergedAt       *time.Time    `json:"mergedAt"`
	BaseRefName    string        `json:"baseRefName"`
	HeadRefOid     string        `json:"headRefOid"`
	Author         *actorPayload `json:"author"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *reviewerPayload `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	TimelineItems struct {
		Nodes []struct {
			Typename          string           `json:"__typename"`
			CreatedAt         time.Time        `json:"createdAt"`
			Actor             *actorPayload    `json:"actor"`
			RequestedReviewer *reviewerPayload `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"timelineItems"`
	Reviews struct {
		Nodes []struct {
			Author      *actorPayload `json:"author"`
			State       string        `json:"state"`
			SubmittedAt *time.Time    `json:"submittedAt"`
			Commit      *struct {
				Oid string `json:"oid"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"reviews"`
}

type searchResponse struct {
	Data struct {
		Search struct {
			IssueCount int `json:"issueCount"`
			PageInfo   struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
			Nodes []pullRequestPayload `json:"nodes"`
		} `json:"search"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func loginOf(actor *actorPayload) string {
	if actor == nil {
		return "ghost"
	}
	return actor.Login
}

func convertReviewer(payload *reviewerPayload) (Reviewer, bool) {
	if payload == nil {
		return Reviewer{}, false
	}
	switch payload.Typename {
	case "User":
		return Reviewer{Kind: ReviewerUser, Name: payload.Login}, true
	case "Team":
		return Reviewer{Kind: ReviewerTeam, Name: payload.Slug}, true
	}
	return Reviewer{}, false
}

func convertPullRequest(payload pullRequestPayload) PullRequest {
	pullRequest := PullRequest{
		Number:         payload.Number,
		Title:          payload.Title,
		Url:            payload.Url,
		Author:         loginOf(payload.Author),
		ReviewDecision: payload.ReviewDecision,
		CreatedAt:      payload.CreatedAt,
		UpdatedAt:      payload.UpdatedAt,
		HeadOid:        payload.HeadRefOid,
		BaseRef:        payload.BaseRefName,
	}
	if payload.MergedAt != nil {
		pullRequest.MergedAt = *payload.MergedAt
	}
	switch {
	case payload.State == "MERGED":
		pullRequest.State = PullRequestMerged
	case payload.State == "CLOSED":
		pullRequest.State = PullRequestClosed
	case payload.IsDraft:
		pullRequest.State = PullRequestDraft
	default:
		pullRequest.State = PullRequestOpen
	}
	for _, node := range payload.ReviewRequests.Nodes {
		if reviewer, ok := convertReviewer(node.RequestedReviewer); ok {
			pullRequest.PendingReviewers = append(pullRequest.PendingReviewers, reviewer)
		}
	}
	for _, node := range payload.TimelineItems.Nodes {
		reviewer, ok := convertReviewer(node.RequestedReviewer)
		if !ok {
			continue
		}
		action := RequestAdded
		if node.Typename == "ReviewRequestRemovedEvent" {
			action = RequestRemoved
		}
		pullRequest.RequestEvents = append(pullRequest.RequestEvents, RequestEvent{
			Reviewer: reviewer, Action: action, At: node.CreatedAt, Actor: loginOf(node.Actor),
		})
	}
	for _, node := range payload.Reviews.Nodes {
		// Unsubmitted (PENDING) reviews have no submittedAt and are invisible to others.
		if node.SubmittedAt == nil {
			continue
		}
		review := Review{Author: loginOf(node.Author), State: node.State, SubmittedAt: *node.SubmittedAt}
		if node.Commit != nil {
			review.CommitOid = node.Commit.Oid
		}
		pullRequest.Reviews = append(pullRequest.Reviews, review)
	}
	return pullRequest
}

func runGraphqlSearch(query string, after string) (searchResponse, error) {
	var response searchResponse
	var searchError error
	for attempt := range 4 {
		time.Sleep(time.Duration(attempt*attempt) * 2 * time.Second)
		if response, searchError = attemptGraphqlSearch(query, after); searchError == nil {
			return response, nil
		}
	}
	return response, searchError
}

func attemptGraphqlSearch(query string, after string) (searchResponse, error) {
	arguments := []string{"api", "graphql", "-f", "query=" + searchQuery, "-f", "q=" + query}
	if after != "" {
		arguments = append(arguments, "-f", "after="+after)
	}
	result := RunCommand(90*time.Second, "", "gh", arguments...)
	var response searchResponse
	if json.Unmarshal([]byte(result.Stdout), &response) != nil {
		return response, fmt.Errorf("gh graphql: %s", strings.TrimSpace(result.Stderr))
	}
	// gh exits non-zero on partial errors (e.g. a team hidden from the token); keep the data.
	if len(response.Data.Search.Nodes) == 0 && len(response.Errors) > 0 {
		return response, errors.New("gh graphql: " + response.Errors[0].Message)
	}
	return response, nil
}

// SearchPullRequests pages through a GitHub search. Past the 1000-result search cap it
// stops after the first page and returns only the count, so the caller can narrow the query.
func SearchPullRequests(query string) ([]PullRequest, int, error) {
	var pullRequests []PullRequest
	after := ""
	for {
		response, searchError := runGraphqlSearch(query, after)
		if searchError != nil {
			return pullRequests, 0, searchError
		}
		issueCount := response.Data.Search.IssueCount
		if issueCount > 1000 {
			return nil, issueCount, nil
		}
		for _, node := range response.Data.Search.Nodes {
			// Search nodes that are not pull requests decode with number 0.
			if node.Number != 0 {
				pullRequests = append(pullRequests, convertPullRequest(node))
			}
		}
		if !response.Data.Search.PageInfo.HasNextPage {
			return pullRequests, issueCount, nil
		}
		after = response.Data.Search.PageInfo.EndCursor
	}
}

// FetchPullRequests reads pull requests by number, bypassing search-index lag.
func FetchPullRequests(repository string, numbers []int) ([]PullRequest, error) {
	owner, name, _ := strings.Cut(repository, "/")
	var fields strings.Builder
	for _, number := range numbers {
		fmt.Fprintf(&fields, "pr%d: pullRequest(number: %d) { ...PullRequestFields }\n", number, number)
	}
	query := fmt.Sprintf("query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) {\n%s} }\n%s", fields.String(), pullRequestFields)
	result := RunCommand(60*time.Second, "", "gh", "api", "graphql", "-f", "query="+query, "-f", "owner="+owner, "-f", "name="+name)
	var response struct {
		Data struct {
			Repository map[string]*pullRequestPayload `json:"repository"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(result.Stdout), &response) != nil || !result.Succeeded() {
		return nil, fmt.Errorf("gh graphql: %s", strings.TrimSpace(result.Stderr))
	}
	var pullRequests []PullRequest
	for _, payload := range response.Data.Repository {
		if payload != nil {
			pullRequests = append(pullRequests, convertPullRequest(*payload))
		}
	}
	return pullRequests, nil
}

func FetchIdentity(organization string) (Identity, error) {
	loginResult := RunCommand(30*time.Second, "", "gh", "api", "user", "--jq", ".login")
	if !loginResult.Succeeded() {
		return Identity{}, fmt.Errorf("gh api user: %s", strings.TrimSpace(loginResult.Stderr))
	}
	teamsResult := RunCommand(60*time.Second, "", "gh", "api", "--paginate", "user/teams",
		"--jq", fmt.Sprintf(`.[] | select(.organization.login == %q) | .slug`, organization))
	if !teamsResult.Succeeded() {
		return Identity{}, fmt.Errorf("gh api user/teams: %s", strings.TrimSpace(teamsResult.Stderr))
	}
	return Identity{
		Login: strings.TrimSpace(loginResult.Stdout),
		Teams: strings.Fields(teamsResult.Stdout),
	}, nil
}
