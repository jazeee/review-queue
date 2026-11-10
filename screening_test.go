package main

import "testing"

func TestScreeningProgressFollowsRoteReviewLogs(t *testing.T) {
	model := Model{screening: &Screening{Progress: map[int]ScreeningProgress{}}}
	review := func() string {
		text := ""
		for _, part := range renderReview(PullRequest{Number: 7}, model.verdicts, model.screening) {
			text += part.text
		}
		return text
	}
	steps := []struct {
		event string
		want  string
	}{
		{"fetching", "⟳ fetch"},
		{"checking out 0123456789ab", "⟳ checkout"},
		{"screening 2100 lines in 2 chunk(s)", "⟳ 0/2"},
		{"chunk 2/2 → approve, high confidence, none risk", "⟳ 1/2"},
		{"could not save verdict: disk full", "⟳ 1/2"},
		{"chunk 1/2 → needs-human, high confidence, low risk", ""},
	}
	for _, step := range steps {
		model.applyScreeningProgress(screeningProgressMsg{Number: 7, Event: step.event})
		if got := review(); got != step.want {
			t.Fatalf("after %q: got %q, want %q", step.event, got, step.want)
		}
	}
}

func TestScreeningProgressFollowsCommentLogs(t *testing.T) {
	model := Model{screening: &Screening{Progress: map[int]ScreeningProgress{7: {Stage: "start"}}}}
	stages := map[string]string{"fetching": "fetch", "drafting comments": "draft", "posting 3 inline comments": "post"}
	for _, event := range []string{"fetching", "drafting comments", "posting 3 inline comments"} {
		model.applyScreeningProgress(screeningProgressMsg{Number: 7, Event: event})
		if got := model.screening.Progress[7].Stage; got != stages[event] {
			t.Fatalf("after %q: stage %q, want %q", event, got, stages[event])
		}
	}
	model.applyScreeningProgress(screeningProgressMsg{Number: 7, Event: "posted https://github.com/o/r/pull/7#pullrequestreview-1"})
	if _, running := model.screening.Progress[7]; running || model.screening.PostedUrl != "https://github.com/o/r/pull/7#pullrequestreview-1" {
		t.Fatalf("posted review not recorded: %+v", model.screening)
	}
}
