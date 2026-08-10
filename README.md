# postmortem

A CLI that builds a merged, chronological incident timeline from multiple
sources — journald, git history, Prometheus — over a given time window.
When something breaks, the first question is usually "what changed?".
`postmortem` answers that by lining up deploys, config changes, and log
events on one timeline instead of tab-switching between four tools.

## Install

```sh
go install github.com/ryankidd/postmortem/cmd/postmortem@latest
```

Or build from source:

```sh
go build -o postmortem ./cmd/postmortem
```

## Usage

```sh
postmortem --since=2h
postmortem --since=2026-08-10T10:00:00Z --until=2026-08-10T12:00:00Z
```

- `--since` — start of the window: an RFC3339 timestamp, or a Go duration
  (`30m`, `2h`) measured back from now. Defaults to `1h`.
- `--until` — end of the window: an RFC3339 timestamp. Defaults to now.

Each line of output is `<timestamp> [<source>] <summary>`, sorted
chronologically across all sources.

This is an early build: the window parsing and timeline merge/render
pipeline are in place and tested against an injectable `Source` interface,
but no real sources (journald, git, Prometheus) are wired in yet, so a run
currently prints an empty timeline. That's next.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```
