package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerdictsOnlyAnswerForTheCurrentHead(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	verdicts := NewVerdicts("owner/repo")
	if makeError := os.MkdirAll(filepath.Dir(verdicts.path), 0o755); makeError != nil {
		t.Fatal(makeError)
	}
	contents := `{
	  "a": {"number": 7, "headOid": "old", "attention": "rote", "evaluatedAt": "2026-11-01T00:00:00Z"},
	  "b": {"number": 7, "headOid": "new", "attention": "light", "evaluatedAt": "2026-11-02T00:00:00Z"},
	  "c": {"number": 7, "headOid": "new", "attention": "deep", "evaluatedAt": "2026-11-03T00:00:00Z"},
	  "d": {"number": 8, "headOid": "head", "evaluatedAt": "2026-11-03T00:00:00Z"}
	}`
	if writeError := os.WriteFile(verdicts.path, []byte(contents), 0o644); writeError != nil {
		t.Fatal(writeError)
	}
	verdicts.Reload()

	if verdict, found := verdicts.Lookup(PullRequest{Number: 7, HeadOid: "new"}); !found || verdict.Attention != "deep" {
		t.Fatalf("want the latest verdict for the current head, got %+v %v", verdict, found)
	}
	if _, found := verdicts.Lookup(PullRequest{Number: 7, HeadOid: "newer"}); found {
		t.Fatal("a verdict for an older head must not describe new commits")
	}
	if _, found := verdicts.Lookup(PullRequest{Number: 8, HeadOid: "head"}); found {
		t.Fatal("skipped or failed screens carry no attention and must not show")
	}
}
