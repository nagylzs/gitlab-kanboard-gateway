# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Two Go binaries sharing one config file and one Kanboard JSON-RPC client, no database, no deps beyond
`go-flags` and `yaml.v3`:

- **`gitlab-kanboard-gateway`** (daemon): receives GitLab **push** webhook events over HTTP, scans commit
  messages for Kanboard task references (configurable regexps), and posts a comment on each referenced task.
- **`kanboard-task`** (CLI, read-only): dumps one or more tasks as agent-friendly JSON (description, comments,
  attachments, subtasks, links, resolved names). Built for AI agents that need to read tickets. It must never
  call a Kanboard procedure that modifies data.

## Commands

```bash
go build ./...                 # compile everything
go vet ./...                   # only static check used (no linter config; vet currently reports 2 slog-arg warnings)
go run ./cmd/gitlab-kanboard-gateway -v -c config.yml   # run locally (needs a real Kanboard + a config file)
go run ./cmd/gitlab-kanboard-gateway --info             # prints example config + template arg docs, then exits
go run ./cmd/kanboard-task -c assets/config.yml 13670   # dump a task as JSON (read-only, safe to run)
go run ./cmd/kanboard-task --info                       # documents every output field
./scripts/build.sh             # build for host OS/arch into dist/<os>/<arch>/ with version ldflags
./scripts/build.sh all         # cross-build linux+windows x amd64+386 into dist/
```

The only tests are in `internal/taskdump` (`go test ./internal/taskdump/ -run TestRewriteEmbeddedFiles`). CI (`.github/workflows/go.yml`) only builds
linux/amd64 and windows/amd64 with the same `-ldflags -X ...internal/version.{Built,Commit,Branch}` pattern that
`scripts/build.sh` uses. If you add a version field or a new `cmd/`, update both (the `cmds=` list in the
script and the per-binary build steps in the workflow).

`assets/config.yml` (a symlink to the developer's real config) points at a live Kanboard server. It is fine to use
it for **read-only** probing (`getTask`, `getAllComments`, ...) while developing, but never call a procedure that
creates, updates or removes anything there.

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

- **`internal/config`** – YAML → `Config`. `LoadConfig` validates the full gateway config;
  `LoadKanboardConfig` reads only the `Kanboard` connection keys for the read-only tool. Note the `*String` fields (`TaskRefsStrings`, `MinRefreshIntervalString`,
  `CommentTemplateString`, …) are the raw YAML values; `LoadConfig` compiles/parses them into the sibling typed
  fields (`TaskRefPatterns`, `MinRefreshInterval`, `CommentTemplate`) and validates them. Add new settings
  following that same raw+parsed pair pattern. CLI flags live here too (`GatewayOpts`, parsed with `go-flags`).
- **`internal/kanboard`** – thin JSON-RPC 2.0 client. `WebClientRpcCall[REQ, RESP]` in `rpc.go` does the
  generic POST with HTTP basic auth (`Username` is always `"jsonrpc"`, `Password` is the API token), checks for
  an RPC `error` object, then unmarshals into the typed response. Each API method is its own file
  (`get_task.go`, `get_all_comments.go`, `create_comment.go`, ...). New read calls use the generic
  `KbRequest[P]` / `KbResponse[R]` envelopes plus a `Kb*IdParam` struct (see `rpc_types.go`); result structs for
  the task-detail procedures live in `rpc_types_details.go`. **Field types follow what the server actually
  returns, not the API docs**: the docs show every value as a string, but Kanboard 1.2.5x returns ints/bools
  (see commits 4af9c6d and 4431f42). Nullable dates are `*int`; "not found" lookups return `result: null`, which
  the wrappers surface as a nil pointer with no error. Empty PHP arrays may serialize as `[]` instead of `{}`
  (tags, metadata), handled by `decodeStringMap`.
- **`internal/taskdump`** – builds the `kanboard-task` output. `model.go` is the JSON schema (also documented
  in the binary's `--info` text; keep the two in sync), `dump.go` does the ~12 read calls per task, resolves
  ids to names (users cached per `Dumper`), converts unix timestamps to RFC3339 and optionally downloads
  attachments to `<dir>/<taskId>/<fileId>-<name>`. A failing `getTask` is fatal; any other failed call is
  appended to `warnings` and the document is still emitted. `parse_id.go` accepts numbers, `#KB123`, and task URLs.
  `comments.go` rewrites the relative `<img src="?controller=FileViewerController...">` tags Kanboard inserts
  for pasted screenshots into markdown images (attachments are therefore collected before comments).
- **`internal/webhooks`** – GitLab push-event payload structs (`pushevent.go`, field comments show example
  values) and the shared `PushQueue`.

The three struct files linked from `--info` (`processor/template_args.go`, `webhooks/pushevent.go`,
`kanboard/rpc_type_task.go`) are the public contract for what users can reference in `CommentTemplate`;
renaming fields there is a breaking config change.

Logging is `log/slog` JSON to stderr; default level is Warn, `-v` → Info, `--debug` → Debug.
