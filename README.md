# review-queue

A terminal inbox for pull requests that asked you, or any of your teams, for
review: the GitHub Slack feed, without duplicates and with read state.

Every PR updated in the last 14 days is scanned through the GitHub GraphQL API
(via `gh`), and kept if a review request ever targeted you or one of your org
teams. Each PR is one row, however many of your teams were requested.

## Build & run

```sh
go vet ./... && go test ./... && CGO_ENABLED=0 go build -o bin/review-queue .
ln -sfn "$PWD/bin/review-queue" ~/.local/bin/review-queue
review-queue                       # VivaaHealth/vivaa, 14 days
review-queue --repo owner/name --days 7
```

`p` creates (or reuses) a worktree of `--checkout` (default `~/code/notable/vivaa`)
under `<primary>/worktrees/` on the PR head branch, or on `pr-<number>` from
`pull/<number>/head` for forks and deleted branches, then opens pi there through
`~/.local/bin/pi-worktree-session.sh` (from worktree-dashboard) in `$TERMINAL`,
using `ptyxis-new-window.sh` for ptyxis when present.

The first run backfills in day-sized windows (~2 minutes); after that it polls
incrementally 30 seconds after each scan finishes. State lives in
`~/.local/state/review-queue/<owner>-<repo>/` (`cache.json`, `dismissed.json`).

## Views

Closed PRs, and PRs merged more than 5 days ago, are hidden everywhere. Drafts are
hidden except in Mine.


| View    | Shows                                                                  |
|---------|------------------------------------------------------------------------|
| Inbox   | Unread: you have not reviewed since the latest request, and not marked seen. Stacked PRs (base not `main`) are hidden until you press `n`; the header counts them |
| Waiting | Open PRs where a request to you or your team is still pending          |
| All     | Every PR that requested you or your teams in the window                |
| Mine    | Your unmerged PRs without a `WIP` label, with their pending reviewers  |

## Columns

- **●** unread.
- **+** added lines, shaded black at ≤1, fading to green at 500, then through
  yellow to red at 1000+, and **-** deleted lines.
- **For** the requested targets that are yours (`@me` or team slugs).
  Yellow = still pending; ~~struck~~ = fulfilled by someone's review or removed.
- **Me** `new`, `re-req` (asked again after your review), `reviewed`,
  `+commits` (reviewed, but the head moved since), or `seen` (dismissed locally).
- **Review** shows the [rote-review](../rote-review) bucket for the PR's current head,
  as a colored badge: `rote` (green, auto-approvable), `light` (yellow), `normal`
  (orange), or `deep` (red). `rote·held` (orange `·held`) means rote, but approval
  waits on the author replying to reviewer feedback. While rote-review works on a PR
  it shows `⟳` with the stage (`fetch`, `checkout`, then chunks done out of total).
  Verdicts for an older head are not shown. The right side of the detail pane shows
  rote-review's summary, standards violations (`✖`), unanswered comments, and standards
  notes (`◦`), word-wrapped.
  `~/.local/state/rote-review/<owner>-<repo>/verdicts.json` is re-read on each poll,
  on focus, and every second while a screen runs, when it has changed.
- **Notes** flags open PRs with merge conflicts (red).
- **Title** is prefixed with the base branch, highlighted yellow, when it is not
  `main` (stacked PRs).
- **Title** is struck through once none of your requests are pending, like Slack.

A review you submit in the GitHub UI marks the PR read on the next poll. PRs
opened with `o` are re-fetched directly (no search-index lag) on every poll for
30 minutes, and immediately when the terminal regains focus (in tmux this needs
`set -g focus-events on`). A
newer review request makes it unread again, even after a local dismissal.

## Keys

| Key | Action |
|-----|--------|
| `j`/`k` `↑`/`↓` `pgup`/`pgdn` `g`/`G` | move |
| `tab` / `shift+tab` / `1`–`4` | switch view |
| `o` / `enter` | open the PR in the browser |
| `p` | open pi (tmux split, new terminal) in a worktree on the PR head branch |
| `s` | screen the selected PR with `rote-review check --always` (re-screens with `--fresh` when its head already has a verdict) |
| `S` | run a full `rote-review scan` (never approves or comments); one rote-review run at a time, shown in the header |
| `space` | toggle seen (local dismissal) |
| `A` `A` | mark every visible row seen |
| `u` | undo the last seen/all-seen |
| `d` | toggle drafts only for the current tab (tab shows ✎) |
| `n` | toggle stacked PRs (base not `main`) for the current tab; hidden by default in Inbox only |
| `/` | filter the current tab (regex, case-insensitive; literal fallback; tab shows /); `esc` clears |
| `y` | copy the PR URL |
| `r` / `R` | refresh now / rescan the whole window (queued if a scan is running) |
| click | select a row (double-click opens it), or switch to a tab; the wheel moves a page |
| `m` | toggle mouse capture |
| `q` | quit |
