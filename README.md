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
postmortem --since=2h --prom-url=http://localhost:9090 --prom-query=node_load1 --prom-threshold=5
```

- `--since` — start of the window: an RFC3339 timestamp, or a Go duration
  (`30m`, `2h`) measured back from now. Defaults to `1h`.
- `--until` — end of the window: an RFC3339 timestamp. Defaults to now.
- `--unit` — restrict journald events to a single systemd unit. Defaults to
  all units.
- `--git-repo` — include commits and tags from a git repository. Repeat the
  flag to pull in several repositories at once. Off by default.
- `--prom-url` — base URL of a Prometheus server to range-query, e.g.
  `http://localhost:9090`. Required to enable the Prometheus source.
- `--prom-query` — a PromQL expression to evaluate over the window. Repeat the
  flag to watch several expressions at once.
- `--prom-threshold` — the value a series has to cross for a crossing to
  become an event. Defaults to `0`.
- `--prom-step` — resolution of the range query. Defaults to `1m`.
- `--format` — output format: `text` (default) or `markdown`.

Each line of the default text output is `<timestamp> [<source>] <summary>`,
sorted chronologically across all sources. Anomalies carry an indented
`suspect` line naming the change that preceded them (see
[Correlation](#correlation)):

```
2026-08-10T12:00:00Z [git] 9f3c1ab Add request timeout to the API client (Ada Lovelace)
2026-08-10T12:10:00Z [prometheus] node_load1: node_load1{instance="web-1"} crossed above 5 (value 6.1)
    suspect: 10m after [git] 9f3c1ab Add request timeout to the API client (Ada Lovelace)
2026-08-10T12:15:00Z [git] tag v1.4.0 -> 5b2d4e7 Release 1.4.0
2026-08-10T12:16:04Z [journald] api.service: upstream timeout after 30s
    suspect: 1m4s after [git] tag v1.4.0 -> 5b2d4e7 Release 1.4.0
```

### Markdown

`--format=markdown` renders the same timeline as a Markdown report you can
paste straight into an incident document: a heading with the window, then a
table of events, with each anomaly's `suspect` surfaced on a second line
inside its row.

```sh
postmortem --since=1h --git-repo=/srv/api --prom-url=http://localhost:9090 \
  --prom-query=node_load1 --prom-threshold=5 --format=markdown
```

```markdown
# Incident timeline

**Window:** 2026-08-10T12:00:00Z → 2026-08-10T13:00:00Z

| Time | Source | Event |
| --- | --- | --- |
| 2026-08-10T12:00:00Z | git | 9f3c1ab Add request timeout to the API client (Ada Lovelace) |
| 2026-08-10T12:10:00Z | prometheus | node_load1: node_load1{instance="web-1"} crossed above 5 (value 6.1)<br>**suspect:** 10m after [git] 9f3c1ab Add request timeout to the API client (Ada Lovelace) |
| 2026-08-10T12:15:00Z | git | tag v1.4.0 -> 5b2d4e7 Release 1.4.0 |
| 2026-08-10T12:16:04Z | journald | api.service: upstream timeout after 30s<br>**suspect:** 1m4s after [git] tag v1.4.0 -> 5b2d4e7 Release 1.4.0 |
```

## Worked example

The git source needs nothing but a readable repository, so the whole flow is
reproducible in a throwaway one. Build a repo with two commits and a release
tag at known times (signing is disabled and the dates are pinned so the short
SHAs below match exactly):

```sh
repo=$(mktemp -d)
git -C "$repo" init -q -b main
git -C "$repo" config user.name "Ada Lovelace"
git -C "$repo" config user.email ada@example.com
git -C "$repo" config commit.gpgsign false

GIT_AUTHOR_DATE=2026-08-10T12:00:00Z GIT_COMMITTER_DATE=2026-08-10T12:00:00Z \
  git -C "$repo" commit -q --allow-empty -m "Add request timeout to the API client"
GIT_AUTHOR_DATE=2026-08-10T12:15:00Z GIT_COMMITTER_DATE=2026-08-10T12:15:00Z \
  git -C "$repo" commit -q --allow-empty -m "Release 1.4.0"
GIT_COMMITTER_DATE=2026-08-10T12:15:00Z \
  git -C "$repo" tag -a v1.4.0 -m "Release 1.4.0"
```

Build the timeline over that window. `--unit` points at a unit that does not
exist, so the example shows only the git source rather than whatever happens to
be in the local journal:

```sh
postmortem --since=2026-08-10T11:59:00Z --until=2026-08-10T12:30:00Z \
  --git-repo="$repo" --unit=postmortem-example.service
```

```
2026-08-10T12:00:00Z [git] a0899d0 Add request timeout to the API client (Ada Lovelace)
2026-08-10T12:15:00Z [git] 9a0f18c Release 1.4.0 (Ada Lovelace)
2026-08-10T12:15:00Z [git] tag v1.4.0 -> 9a0f18c Release 1.4.0
```

Add `--format=markdown` for a report that pastes straight into an incident doc:

```markdown
# Incident timeline

**Window:** 2026-08-10T11:59:00Z → 2026-08-10T12:30:00Z

| Time | Source | Event |
| --- | --- | --- |
| 2026-08-10T12:00:00Z | git | a0899d0 Add request timeout to the API client (Ada Lovelace) |
| 2026-08-10T12:15:00Z | git | 9a0f18c Release 1.4.0 (Ada Lovelace) |
| 2026-08-10T12:15:00Z | git | tag v1.4.0 -> 9a0f18c Release 1.4.0 |
```

Drop the `--unit` filter, or add `--prom-url`/`--prom-query`, to fold journald
entries and Prometheus threshold crossings into the same timeline. The
`suspect` annotations shown elsewhere in this README appear once there is an
anomaly for a change to be correlated against — a git-only window has changes
but nothing that broke.

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

**prometheus** — a range query (`query_range`) over the same window for each
`--prom-query` expression. Wherever a returned series crosses `--prom-threshold`
— in either direction — an event is emitted at the crossing sample, labelled
with the series that crossed and the value it reached. This talks to the
Prometheus HTTP API, so `--prom-url` needs to point at a reachable server.

## Correlation

Every event is classified as either a **change** (something that altered the
system) or an **anomaly** (something that broke), or left unclassified:

- **changes** — git commits and tags, and journald lines where systemd
  reports a unit starting, stopping, restarting or reloading.
- **anomalies** — Prometheus threshold crossings, and journald lines logged
  at syslog priority `err` or worse.

After the timeline is merged, a correlation pass walks it in order and, for
each anomaly, finds the nearest change that occurred strictly before it. That
change is attached to the anomaly as a `suspect` annotation, along with the
gap between the two (`12m after [git] 9f3c1ab …`). An anomaly with no change
ahead of it is left unannotated.

This is a **heuristic, not a proof of causation**. Naming the nearest
preceding change is a correlation only: a change that merely happened to land
just before an anomaly is flagged whether or not it caused it, and a genuine
cause that predates a closer, unrelated change is missed. Treat a suspect as
a lead to investigate, not a verdict.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```
