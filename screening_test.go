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
