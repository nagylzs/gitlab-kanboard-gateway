# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A small Go daemon that receives GitLab **push** webhook events over HTTP, scans commit messages for Kanboard
task references (configurable regexps), and posts a comment on each referenced Kanboard task via Kanboard's
JSON-RPC API. Single binary, single config file, no database. No external deps beyond `go-flags` and `yaml.v3`.

## Commands

```bash
go build ./...                 # compile everything
go vet ./...                   # only static check used (no linter config; vet currently reports 2 slog-arg warnings)
go run ./cmd/gitlab-kanboard-gateway -v -c config.yml   # run locally (needs a real Kanboard + a config file)
go run ./cmd/gitlab-kanboard-gateway --info             # prints example config + template arg docs, then exits
./scripts/build.sh             # build for host OS/arch into dist/<os>/<arch>/ with version ldflags
./scripts/build.sh all         # cross-build linux+windows x amd64+386 into dist/
```

There are no tests in the repo (`go test ./...` finds nothing). CI (`.github/workflows/go.yml`) only builds
linux/amd64 and windows/amd64 with the same `-ldflags -X ...internal/version.{Built,Commit,Branch}` pattern that
`scripts/build.sh` uses. If you add a version field, update both.

`assets/config.yml` is gitignored on purpose: that's where the developer keeps a real config with credentials.
`assets/config_example.yml` is the committed example and is `go:embed`ded into the binary for `--info`, so keep
it valid YAML and keep its comments accurate; it doubles as the user-facing config documentation.

## Architecture

Three goroutines wired together in `cmd/gitlab-kanboard-gateway/gitlab-kanboard-gateway.go`, communicating
through one global channel:

1. **`internal/websrv`** – `net/http` server on `Webhook.ListenAddress`. Rejects non-POST, checks the
   `X-Gitlab-Token` header against `Webhook.SecretToken` (skipped if empty), decodes the body as
   `webhooks.PushEvent`, rejects anything whose `event_name != "push"`, sets `CanRetry = true`, and pushes the
   event into `webhooks.PushQueue` (buffered chan, cap 1000, 1s enqueue timeout → HTTP 500 if full).
2. **`internal/processor`** – single consumer loop (`Processor.Run`): each iteration calls `refreshCache()` then
   `runOnce()` (which blocks up to 1s waiting on the queue).
   - `refreshCache` reloads *all* projects and *all* tasks of every project from Kanboard into an in-memory
     `map[taskId]KbResponseTask`. It runs at most every `DefRefreshInterval`, or every `MinRefreshInterval`
     if there are events waiting for retry, and is self-throttled to ≥10× the last refresh duration.
   - `runOnce` filters the event's `Ref` against `Kanboard.Refs` patterns, then applies each `Kanboard.TaskRefs`
     regexp to every commit message. **Capture group 1 must be the integer task id.** One comment per
     (commit, task) pair even if a commit references the same task twice. If a referenced task id is not in the
     cache and `CanRetry` is set, the whole event is parked in `afterRefresh` and re-queued once after the next
     cache refresh (with `CanRetry = false`), so a task created just before the push still gets its comment.
   - Comment text comes from the `CommentTemplate` (Go `text/template`, parsed once at config load) executed
     with `processor.TemplateArgs{Event, Commit, Task}`; on template error it falls back to a hard-coded
     message. Output must be Markdown (Kanboard renders comments as Markdown).
3. **`internal/signal`** – SIGINT/SIGTERM → atomic stop flag polled by `main` and the processor loop. A bad
   regexp match (no capture group / non-integer) calls `signal.Stop(1)` and terminates the whole process.

Supporting packages:

- **`internal/config`** – YAML → `Config`. Note the `*String` fields (`TaskRefsStrings`, `MinRefreshIntervalString`,
  `CommentTemplateString`, …) are the raw YAML values; `LoadConfig` compiles/parses them into the sibling typed
  fields (`TaskRefPatterns`, `MinRefreshInterval`, `CommentTemplate`) and validates them. Add new settings
  following that same raw+parsed pair pattern. CLI flags live here too (`GatewayOpts`, parsed with `go-flags`).
- **`internal/kanboard`** – thin JSON-RPC 2.0 client. `WebClientRpcCall[REQ, RESP]` in `rpc.go` does the
  generic POST with HTTP basic auth (`Username` is always `"jsonrpc"`, `Password` is the API token), checks for
  an RPC `error` object, then unmarshals into the typed response. Each API method is its own file
  (`get_all_projects.go`, `get_all_tasks.go`, `create_comment.go`) with request/response structs in
  `rpc_types.go` / `rpc_type_task.go`. Kanboard returns many fields as strings or nulls, so the task struct uses
  pointers/`interface{}` for those; recent commits were mostly fixing these JSON types against real responses.
- **`internal/webhooks`** – GitLab push-event payload structs (`pushevent.go`, field comments show example
  values) and the shared `PushQueue`.

The three struct files linked from `--info` (`processor/template_args.go`, `webhooks/pushevent.go`,
`kanboard/rpc_type_task.go`) are the public contract for what users can reference in `CommentTemplate`;
renaming fields there is a breaking config change.

Logging is `log/slog` JSON to stderr; default level is Warn, `-v` → Info, `--debug` → Debug.
