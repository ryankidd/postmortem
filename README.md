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
postmortem --since=1h --unit=api.service
```

- `--since` — start of the window: an RFC3339 timestamp, or a Go duration
  (`30m`, `2h`) measured back from now. Defaults to `1h`.
- `--until` — end of the window: an RFC3339 timestamp. Defaults to now.
- `--unit` — restrict journald events to a single systemd unit. Defaults to
  all units.

Each line of output is `<timestamp> [<source>] <summary>`, sorted
chronologically across all sources. It runs `journalctl` under the hood, so
it needs to run somewhere with access to the journal it's reading.

This is an early build: journald is the only source wired in so far. git
history and Prometheus are next, followed by a correlation pass that
annotates anomalies with the nearest preceding change.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```
