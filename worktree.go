package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// EnsurePullRequestWorktree returns a worktree of checkout on the pull request's head,
// reusing one that already has the branch checked out.
func EnsurePullRequestWorktree(checkout string, pullRequest PullRequest) (string, error) {
	git := func(timeout time.Duration, arguments ...string) (string, error) {
		result := RunCommand(timeout, checkout, "git", arguments...)
		if !result.Succeeded() {
			return "", fmt.Errorf("git %s: %s", arguments[0], strings.TrimSpace(result.Stderr))
		}
		return result.Stdout, nil
	}

	pullBranch := fmt.Sprintf("pr-%d", pullRequest.Number)
	porcelain, listError := git(30*time.Second, "worktree", "list", "--porcelain")
	if listError != nil {
		return "", listError
	}
	primary := ""
	worktreePath := ""
	for _, line := range strings.Split(porcelain, "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok {
			if primary == "" {
				primary = path
			}
			worktreePath = path
		}
		if line == "branch refs/heads/"+pullBranch || (!pullRequest.IsCrossRepository && line == "branch refs/heads/"+pullRequest.HeadRef) {
			return worktreePath, nil
		}
	}

	name := strings.Trim(regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(pullRequest.HeadRef, "-"), "-")
	path := filepath.Join(primary, "worktrees", name)
	if _, statError := os.Stat(path); statError == nil {
		path = fmt.Sprintf("%s-pr-%d", path, pullRequest.Number)
	}

	if !pullRequest.IsCrossRepository {
		if _, fetchError := git(120*time.Second, "fetch", "origin", pullRequest.HeadRef); fetchError == nil {
			if _, verifyError := git(10*time.Second, "rev-parse", "--verify", "--quiet", "refs/heads/"+pullRequest.HeadRef); verifyError == nil {
				_, addError := git(120*time.Second, "worktree", "add", path, pullRequest.HeadRef)
				return path, addError
			}
			_, addError := git(120*time.Second, "worktree", "add", "--track", "-b", pullRequest.HeadRef, path, "origin/"+pullRequest.HeadRef)
			return path, addError
		}
	}
	// Forks and deleted head branches (e.g. after merge) are only reachable through GitHub's pull ref.
	if _, fetchError := git(120*time.Second, "fetch", "origin", fmt.Sprintf("+pull/%d/head:%s", pullRequest.Number, pullBranch)); fetchError != nil {
		return "", fetchError
	}
	_, addError := git(120*time.Second, "worktree", "add", path, pullBranch)
	return path, addError
}
