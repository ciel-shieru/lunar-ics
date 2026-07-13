# AGENTS.md — lunar-ics

A small, offline Go program that generates an iCalendar (.ics) feed of Guanyin (觀音 / Avalokiteśvara) prayer observance dates for the current year plus the next two years. Dates are derived from the Chinese lunisolar calendar via `lunar-go` and served over HTTP — no disk writes, no outbound network at runtime.

## Go environment (required)

This repo uses Go installed via Homebrew/Linuxbrew with a non-standard GOPATH. Every bash invocation must set:

```bash
export GOROOT="/home/linuxbrew/.linuxbrew/opt/go/libexec" \
      GOPATH="/tmp/gopath" \
      PATH="$GOROOT/bin:$PATH" \
      HOME="/home/openclaw"
```

Without these, `go` commands fail (`GOPATH entry is relative`).

## Commands

| Step | Command |
|---|---|
| Lint / static check | `go vet ./...` |
| Build binary | `go build -o lunar-ics .` |
| Run tests | `go test -v -count=1 ./...` |
| Verify changes work | `go run . --addr :9000` (confirm HTTP response, then Ctrl+C) |

All commands run from the repo root. The module is `lunar-ics`. Changes are not considered "working" until they have been verified with `go run`.

## Architecture

Single-package Go program (`package main`). Every `.go` file lives at the root level:

```
config.go      — CLI flag parsing (--addr, --pray-start/end, --tz, --guanyin-zhai)
events.go      — Lunar→Gregorian conversion via lunar-go; event generation + deduplication
ics.go         — Hand-rolled RFC 5545 emitter (line folding, CRLF, escaping)
server.go      — Single http.Handler serving precomputed []byte from memory
main.go        — Wires config → events → ICS → HTTP server; embeds tzdata
main_test.go   — 28 tests across config, golden conversions, cross-checks, ICS validity, event generation, server routing
```

The binary (~9MB) includes the embedded IANA timezone database via `_ "time/tzdata"` so `Asia/Shanghai` resolves without host zoneinfo files. No disk writes, no outbound network at runtime.

## Dependencies

Exactly one third-party dependency: `github.com/6tail/lunar-go v1.4.6`, pinned in go.sum with both content hash and module declaration hash. Do not add further dependencies — the ADR explicitly rejects ICS libraries and web frameworks for this project.
