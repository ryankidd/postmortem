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
postmortem --since=2h --git-repo=/srv/api --git-repo=/srv/config
```

- `--since` — start of the window: an RFC3339 timestamp, or a Go duration
  (`30m`, `2h`) measured back from now. Defaults to `1h`.
- `--until` — end of the window: an RFC3339 timestamp. Defaults to now.
- `--unit` — restrict journald events to a single systemd unit. Defaults to
  all units.
- `--git-repo` — include commits and tags from a git repository. Repeat the
  flag to pull in several repositories at once. Off by default.

Each line of output is `<timestamp> [<source>] <summary>`, sorted
chronologically across all sources:

```
2026-08-10T12:00:00Z [git] 9f3c1ab Add request timeout to the API client (Ada Lovelace)
2026-08-10T12:15:00Z [git] tag v1.4.0 -> 5b2d4e7 Release 1.4.0
2026-08-10T12:16:04Z [journald] api.service: upstream timeout after 30s
```

## Sources

**journald** — every journal entry in the window, or just one unit's with
`--unit`. Entries are labelled with their systemd unit, falling back to the
syslog identifier. This shells out to `journalctl`, so it has to run
somewhere with access to the journal it is reading.

**git** — every commit whose commit date lands in the window, rendered as
short SHA, subject, and author, plus any tag created in the window with the
commit it points at. Annotated tags report the commit they dereference to
rather than the SHA of the tag object. This shells out to `git`, so the
paths given to `--git-repo` need to be readable repositories on the machine
running the command.

Prometheus is next, followed by a correlation pass that annotates anomalies
with the nearest preceding change.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```
