package main

import (
	"testing"
	"time"
)

var (
	viewer   = Identity{Login: "jazeee", Teams: []string{"security", "pod-operate"}}
	baseTime = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	security = Reviewer{Kind: ReviewerTeam, Name: "security"}
	me       = Reviewer{Kind: ReviewerUser, Name: "jazeee"}
)

func at(hours int) time.Time {
	return baseTime.Add(time.Duration(hours) * time.Hour)
}

func TestTeamRequestFulfilledByTeammateStaysUnread(t *testing.T) {
	pullRequest := PullRequest{
		Author: "alice", HeadOid: "head",
		RequestEvents: []RequestEvent{
			{Reviewer: security, Action: RequestAdded, At: at(0), Actor: "alice"},
			{Reviewer: Reviewer{Kind: ReviewerTeam, Name: "infrastructure"}, Action: RequestAdded, At: at(0)},
		},
		Reviews: []Review{
			{Author: "copilot-pull-request-reviewer", State: "COMMENTED", SubmittedAt: at(1)},
			{Author: "bob", State: "APPROVED", SubmittedAt: at(2), CommitOid: "head"},
		},
	}
	row, ok := BuildQueueRow(pullRequest, viewer, time.Time{})
	if !ok || len(row.Targets) != 1 {
		t.Fatalf("expected one owned target, got %+v", row.Targets)
	}
	if row.Targets[0].Status != TargetFulfilled || row.Targets[0].ResolvedBy != "bob" {
		t.Fatalf("expected fulfilled by bob, got %+v", row.Targets[0])
	}
	if !row.Settled() || !row.Unread || row.MyStatus != MyStatusUnreviewed {
		t.Fatalf("expected settled unread new row, got %+v", row)
	}
}

func TestMyReviewStatusFollowsRequestsAndHead(t *testing.T) {
	pullRequest := PullRequest{
		Author: "alice", HeadOid: "first",
		PendingReviewers: []Reviewer{me},
		RequestEvents:    []RequestEvent{{Reviewer: me, Action: RequestAdded, At: at(0)}},
		Reviews:          []Review{{Author: "jazeee", State: "CHANGES_REQUESTED", SubmittedAt: at(1), CommitOid: "first"}},
	}
	if row, _ := BuildQueueRow(pullRequest, viewer, time.Time{}); row.MyStatus != MyStatusReviewed || row.Unread {
		t.Fatalf("expected reviewed, got %+v", row)
	}

	pullRequest.HeadOid = "second"
	if row, _ := BuildQueueRow(pullRequest, viewer, time.Time{}); row.MyStatus != MyStatusUpdated || row.Unread {
		t.Fatalf("expected +commits, got %+v", row)
	}

	pullRequest.RequestEvents = append(pullRequest.RequestEvents, RequestEvent{Reviewer: me, Action: RequestAdded, At: at(3)})
	if row, _ := BuildQueueRow(pullRequest, viewer, time.Time{}); row.MyStatus != MyStatusRerequested || !row.Unread {
		t.Fatalf("expected re-requested unread, got %+v", row)
	}
}

func TestDismissalIsUndoneByANewerRequest(t *testing.T) {
	pullRequest := PullRequest{
		Author:        "alice",
		RequestEvents: []RequestEvent{{Reviewer: security, Action: RequestAdded, At: at(0)}},
	}
	if row, _ := BuildQueueRow(pullRequest, viewer, at(1)); row.Unread {
		t.Fatalf("expected dismissed row to be read")
	}
	pullRequest.RequestEvents = append(pullRequest.RequestEvents, RequestEvent{Reviewer: security, Action: RequestAdded, At: at(2)})
	if row, _ := BuildQueueRow(pullRequest, viewer, at(1)); !row.Unread {
		t.Fatalf("expected re-request after dismissal to be unread")
	}
}

func TestRemovedRequestIsNotCreditedToALaterReviewer(t *testing.T) {
	pullRequest := PullRequest{
		Author: "alice",
		RequestEvents: []RequestEvent{
			{Reviewer: security, Action: RequestAdded, At: at(0)},
			{Reviewer: security, Action: RequestRemoved, At: at(1), Actor: "alice"},
		},
		Reviews: []Review{{Author: "bob", State: "APPROVED", SubmittedAt: at(2)}},
	}
	row, _ := BuildQueueRow(pullRequest, viewer, time.Time{})
	if row.Targets[0].Status != TargetRemoved || row.Targets[0].ResolvedBy != "alice" {
		t.Fatalf("expected removed by alice, got %+v", row.Targets[0])
	}
}

func TestDraftsReachInboxOnlyInDraftsMode(t *testing.T) {
	pullRequest := PullRequest{
		Author: "alice", State: PullRequestDraft, BaseRef: "main",
		RequestEvents: []RequestEvent{{Reviewer: security, Action: RequestAdded, At: at(0)}},
	}
	row, _ := BuildQueueRow(pullRequest, viewer, time.Time{})
	if belongsToView(row, ViewInbox, false) || !belongsToView(row, ViewInbox, true) {
		t.Fatalf("expected an unread draft only in drafts mode")
	}

	mine, _ := BuildQueueRow(PullRequest{Author: "jazeee", State: PullRequestDraft, BaseRef: "main"}, viewer, time.Time{})
	if !belongsToView(mine, ViewMine, false) {
		t.Fatalf("expected my draft to stay in Mine outside drafts mode")
	}
}
