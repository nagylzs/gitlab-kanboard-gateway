# gitlab-kanboard-gateway

Connects GitLab to [Kanboard](https://kanboard.org/): when someone pushes commits whose messages reference
Kanboard tasks, a comment with the commit details is added to each referenced task.

The repository builds two programs:

| Binary                    | Purpose                                                                                        |
|---------------------------|------------------------------------------------------------------------------------------------|
| `gitlab-kanboard-gateway` | Background service. Receives GitLab *push* webhooks and creates Kanboard comments from them.   |
| `kanboard-task`           | Command-line tool. Dumps a Kanboard task as JSON, read-only. Made for AI agents and scripts.   |

## How the gateway works

1. GitLab sends a *push event* webhook to the gateway's HTTP endpoint.
2. The gateway checks the `X-Gitlab-Token` header against the configured secret.
3. The pushed branch (`refs/heads/...`) is matched against the `Refs` patterns. Non-matching pushes are ignored.
4. Every commit message is searched with the `TaskRefs` regular expressions. Each match yields a task number.
5. For every (commit, task) pair a Markdown comment is rendered from `CommentTemplate` and posted to the task
   through the Kanboard JSON-RPC API, on behalf of a technical Kanboard user.

The gateway keeps an in-memory cache of all tasks of all projects the technical user can see. The cache is
refreshed every `DefRefreshInterval`. If a commit references a task that is not in the cache (for example the
task was created seconds before the push), the event is retried once after a faster refresh (`MinRefreshInterval`).

Only push events are supported. Other webhook event types are rejected with HTTP 400.

## Requirements

* A GitLab instance that can reach the gateway over HTTP.
* A Kanboard instance with the API enabled (Settings → API).
* Go 1.22 or newer, if you build from source.

## Installation

### Pre-built binaries

Every push to `main` builds Linux and Windows (amd64) binaries as GitHub Actions artifacts. Download them from
the *Actions* tab of this repository.

### Building from source

```bash
git clone https://github.com/nagylzs/gitlab-kanboard-gateway.git
cd gitlab-kanboard-gateway
./scripts/build.sh          # binaries for the host OS/architecture in dist/<os>/<arch>/
./scripts/build.sh all      # linux + windows, amd64 + 386
```

Or simply:

```bash
go build ./cmd/gitlab-kanboard-gateway
go build ./cmd/kanboard-task
```

## Setup

### 1. Kanboard

* Create a regular Kanboard user, for example `gitbot`. The comments will be posted in the name of this user.
  Note its numeric user id (visible in the URL of the user's profile page); it goes into `Kanboard.UserId`.
* Add this user as a member of every project that should receive commit comments. Tasks of projects the user
  is not a member of are invisible to the gateway, so no comments can be added there.
* Open *Settings → API* in Kanboard and note the API endpoint URL (ends with `jsonrpc.php`) and the API token.
  They go into `Kanboard.ApiUrl` and `Kanboard.Password`. `Kanboard.Username` must always be `jsonrpc`.

### 2. GitLab

* Open the GitLab project, go to *Settings → Webhooks*.
* Add a webhook with the URL of the gateway, for example `http://gateway.example.com:8888/`, and enable
  *Push events* only.
* Set a *Secret token*. GitLab sends it in the `X-Gitlab-Token` header; put the same value into
  `Webhook.SecretToken`.
* If the gateway runs on a private network address, a GitLab administrator may have to allow it under
  *Admin Area → Settings → Network → Outbound requests*.

### 3. Configuration file

Print the annotated example configuration and use it as a starting point:

```bash
gitlab-kanboard-gateway --info > config.yml
```

The same command also prints the fields that can be used in the comment template.
The configuration file has three sections:

| Section           | Keys                                                                                                   |
|-------------------|--------------------------------------------------------------------------------------------------------|
| `Kanboard`        | `ApiUrl`, `Username`, `Password`, `UserId`, `TaskRefs`, `Refs`, `MinRefreshInterval`, `DefRefreshInterval` |
| `Webhook`         | `ListenAddress` (host:port to listen on), `SecretToken`                                                |
| `CommentTemplate` | A Go [`text/template`](https://pkg.go.dev/text/template) that renders the Markdown comment.            |

`TaskRefs` is a list of regular expressions applied to every commit message. The first capturing group must
contain the task number. The example configuration recognizes task URLs and `#KB123` style references:

```yaml
TaskRefs:
  - "https://your.kanboard.com/task/(\\d+)"
  - "#[kK][bB](\\d+)\\b"
```

`Refs` is a list of regular expressions; at least one must match the pushed ref (e.g. `refs/heads/main`)
for the push to be processed. Use `".*"` to accept every branch and tag.

The configuration file contains the Kanboard API token and the webhook secret. Keep it readable only by the
user that runs the gateway.

### 4. First run

Start the gateway in the foreground with verbose logging:

```bash
gitlab-kanboard-gateway -v -c config.yml
```

The log (JSON lines on stderr) should show the projects and tasks being loaded from Kanboard. Then:

1. Use the *Test → Push events* button of the GitLab webhook and check that the gateway logs the event.
2. Push a real commit whose message references an existing task and check that the comment appears.

If something does not work, start with `-d` (debug logging), which also shows why an event or a commit
was skipped.

### 5. Running as a service

Run the gateway as an unprivileged user, never as root. On Linux, a minimal systemd unit looks like this:

```ini
[Unit]
Description=GitLab to Kanboard gateway
After=network-online.target

[Service]
User=gitbot
ExecStart=/opt/gitlab-kanboard-gateway/gitlab-kanboard-gateway -c /etc/gitlab-kanboard-gateway/config.yml
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

On Windows, a service wrapper such as [NSSM](https://nssm.cc/) can be used.

The gateway speaks plain HTTP. If GitLab reaches it over an untrusted network, put it behind a reverse
proxy that terminates TLS.

### Command-line options

```
gitlab-kanboard-gateway [OPTIONS]

  -c, --config=  Config file path (required unless --info or --version is given)
  -v, --verbose  Info-level logging (default: warnings only)
  -d, --debug    Debug-level logging
  -i, --info     Print program description, template fields and the example config, then exit
      --version  Print build time, branch and commit, then exit
  -h, --help     Show help
```

## kanboard-task: read-only task dump

`kanboard-task` looks up one or more Kanboard tasks and prints everything about them as JSON on stdout:
title and description, project, column, swimlane and category names, owner and creator, dates, tags,
subtasks, comments, attachments, links to other tasks, and external links. It only ever reads from
Kanboard, so it is safe to hand to an AI agent or a script that needs the content of a ticket.

It uses the same configuration file as the gateway, but reads only `Kanboard.ApiUrl`, `Kanboard.Username`
and `Kanboard.Password`. A file containing just those three keys is sufficient. The path can also be given
in the `KANBOARD_TASK_CONFIG` environment variable.

```bash
kanboard-task -c config.yml 13670                    # one task -> JSON object
kanboard-task -c config.yml 13670 13671              # several tasks -> JSON array
kanboard-task -c config.yml '#KB13670'               # #KB references and task URLs are accepted too
kanboard-task -c config.yml -o ./attachments 13670   # also download attachments to ./attachments/13670/
kanboard-task --info                                 # documents every field of the output
```

Notable output details:

* Ids are resolved to names, timestamps are RFC 3339 strings, `null` means "not set".
* Comments are Markdown. Screenshots pasted into a comment are stored by Kanboard as task attachments
  and embedded as relative `<img>` tags; these are rewritten to Markdown images that point at the
  downloaded file (with `-o`) or at the absolute Kanboard URL. The comment's `attachment_ids` lists the
  attachments it embeds.
* Attachments larger than `--max-download-bytes` (default 20 MiB) are listed but not downloaded.
* If a secondary lookup fails (for example the comments could not be loaded), the document is still
  printed and the problem is recorded in its `warnings` list.

Logs go to stderr; stdout carries only JSON. Exit codes:

| Code | Meaning                                              |
|------|------------------------------------------------------|
| 0    | Success                                              |
| 1    | Usage or configuration error                         |
| 2    | A task does not exist (others are still printed)     |
| 3    | Kanboard API error                                   |

## Development

```bash
go build ./...      # compile
go vet ./...        # static checks
go test ./...       # unit tests
```

Both binaries take their `--version` information from `-ldflags -X` values set by `scripts/build.sh` and by
the GitHub Actions workflow.

## License

Apache License 2.0, see [LICENSE](LICENSE).
