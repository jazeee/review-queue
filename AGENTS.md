# Agent instructions

## Always rebuild the installed binary

`~/.local/bin/review-queue` is a symlink to `bin/review-queue` in this repo.
After **any** source change, finish with:

```sh
go vet ./... && go test ./... && CGO_ENABLED=0 go build -o bin/review-queue .
```

Tell the user to restart review-queue to pick up the new build.
