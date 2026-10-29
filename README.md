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

The first run backfills in day-sized windows (~2 minutes); after that it polls
incrementally 30 seconds after each scan finishes. State lives in
`~/.local/state/review-queue/<owner>-<repo>/` (`cache.json`, `dismissed.json`).

## Views

Closed PRs, and PRs merged more than 5 days ago, are hidden everywhere. Drafts and
stacked PRs (base is not `main`) are hidden except in Mine.


| View    | Shows                                                                  |
|---------|------------------------------------------------------------------------|
| Inbox   | Unread: you have not reviewed since the latest request, and not marked seen |
| Waiting | Open PRs where a request to you or your team is still pending          |
| All     | Every PR that requested you or your teams in the window                |
| Mine    | PRs you authored, with their pending reviewers                         |

## Columns

- **●** unread.
- **Lines** additions + deletions, black at ≤1, fading to green at 500,
  then through yellow to red at 1000+, then
  **+** additions and **-** deletions.
- **For** the requested targets that are yours (`@me` or team slugs).
  Yellow = still pending; ~~struck~~ = fulfilled by someone's review or removed.
- **Me** `new`, `re-req` (asked again after your review), `reviewed`,
  `+commits` (reviewed, but the head moved since), or `seen` (dismissed locally).
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
| `space` | toggle seen (local dismissal) |
| `A` `A` | mark every visible row seen |
| `u` | undo the last seen/all-seen |
| `/` | filter (regex, case-insensitive; literal fallback); `esc` clears |
| `y` | copy the PR URL |
| `r` / `R` | refresh now / rescan the whole window (queued if a scan is running) |
| `m` | toggle mouse capture |
| `q` | quit |
