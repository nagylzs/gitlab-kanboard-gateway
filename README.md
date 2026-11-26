# gitlab-kanboard-gateway

Connects GitLab to [Kanboard](https://kanboard.org/): when someone pushes commits whose messages reference
Kanboard tasks, a comment with the commit details is added to each referenced task.

The repository builds three programs:

| Binary                    | Purpose                                                                                        |
|---------------------------|------------------------------------------------------------------------------------------------|
| `gitlab-kanboard-gateway` | Background service. Receives GitLab *push* webhooks and creates Kanboard comments from them.   |
| `kanboard-task`           | Command-line tool. Dumps a Kanboard task as JSON. Made for AI agents and scripts.              |
| `kanboard-mcp`            | MCP server. Gives AI agents the same access to Kanboard tasks as MCP tools.                    |

Both are read-only unless they are started with `--writable`; then they can also create tasks, comment,
upload files, link, move and close tasks (see [Changing tasks](#changing-tasks---writable)).

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
* Go 1.25 or newer, if you build from source.

### Compatibility

Tested with Kanboard v1.2.54. Older releases returned most API values as strings (for example `"id": "13"`,
`"is_active": "1"`) and are not supported: the response types in this code follow what current Kanboard
returns, which differs from the string-typed examples still shown in the Kanboard API documentation.

## Installation

### Pre-built binaries

Every push to `main` builds Linux and Windows (amd64) and macOS (arm64) binaries as GitHub Actions artifacts.
Download them from the *Actions* tab of this repository.

The archives attached to each [release](https://github.com/nagylzs/gitlab-kanboard-gateway/releases) contain
`dist/<os>/<arch>/` for Linux and Windows (amd64, 386) and, from v1.5.0 on, macOS (amd64, arm64). The macOS
binaries are not notarized, so macOS refuses to run them after a browser download; remove the quarantine flag
first:

```bash
xattr -d com.apple.quarantine gitlab-kanboard-gateway kanboard-task kanboard-mcp
```

### Building from source

```bash
git clone https://github.com/nagylzs/gitlab-kanboard-gateway.git
cd gitlab-kanboard-gateway
./scripts/build.sh          # binaries for the host OS/architecture in dist/<os>/<arch>/
./scripts/build.sh all      # linux + windows (amd64, 386), macOS (amd64, arm64)
```

Or simply:

```bash
go build ./cmd/gitlab-kanboard-gateway
go build ./cmd/kanboard-task
go build ./cmd/kanboard-mcp
```

## Setup

### 1. Kanboard

* Create a regular Kanboard user, for example `gitbot`. The comments will be posted in the name of this user.
  Note its numeric user id (visible in the URL of the user's profile page); it goes into `Kanboard.UserId`.
* Add this user as a member of every project that should receive commit comments. Tasks of projects the user
  is not a member of are invisible to the gateway, so no comments can be added there.
* Open *Settings → API* in Kanboard and note the API endpoint URL (ends with `jsonrpc.php`) and the API token.
  They go into `Kanboard.ApiUrl` and `Kanboard.Password`. For the gateway, `Kanboard.Username` must be
  `jsonrpc`. (`kanboard-task` can also use a personal token, see below.)

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

## kanboard-task: task dump

`kanboard-task` looks up one or more Kanboard tasks and prints everything about them as JSON on stdout:
title and description, project, column, swimlane and category names, owner and creator, dates, tags,
subtasks, comments, attachments, links to other tasks, and external links. Without `--writable` it only
ever reads from Kanboard, so it is safe to hand to an AI agent or a script that needs the content of a ticket.

It uses the same configuration file as the gateway, but reads only `Kanboard.ApiUrl`, `Kanboard.Username`
and `Kanboard.Password`. A file containing just those three keys is sufficient. The path can also be given
in the `KANBOARD_TASK_CONFIG` environment variable.

Instead of the application API token (`Username: "jsonrpc"`), `kanboard-task` can log in as a Kanboard user:
set `Username` to the user's login name and `Password` to a personal access token created under
*My profile → API*. It then reads with that user's permissions:

* Only projects the user is a member of are accessible, even if the user is a Kanboard administrator.
  Other tasks fail with an API error (403), and `--search` searches only the user's projects.
* For a non-administrator, `owner` and `creator` contain only the id and the display name, because
  looking up a user (`getUser`) is admin-only. The name is taken from the project's member list.
* Comments restricted to managers or administrators are left out for users below that role.
* `assignee:me` and similar `me` filters work in `--search` queries.

With the [Group_assign](https://github.com/creecros/Group_assign) plugin on the Kanboard server, a task's
*Assigned Group* and *Other Assignees* appear as `assigned_group` and `other_assignees`, only on tasks that
have them. Listing the other assignees needs version 1.8.4 or newer of the plugin (with an older one, a
warning says so). For a non-administrator the group has only its id, because looking up a group
(`getGroup`) is admin-only. Without the plugin nothing changes. In searches, `allassignees:<username>` (a
filter of the plugin) also finds the tasks where the user is one of the other assignees or a member of the
assigned group.

```bash
kanboard-task -c config.yml 13670                    # one task -> JSON object
kanboard-task -c config.yml 13670 13671              # several tasks -> JSON array
kanboard-task -c config.yml '#KB13670'               # #KB references and task URLs are accepted too
kanboard-task -c config.yml -o ./attachments 13670   # also download attachments to ./attachments/13670/
kanboard-task --info                                 # documents every field of the output
```

### Searching

With `-s/--search QUERY` it runs Kanboard's `searchTasks` instead and prints one JSON object with a short
summary of each matching task (id, url, title, status, project, column, assignee, dates, counters and the
first 300 characters of the description), most recently modified first. `QUERY` uses the search syntax of
the Kanboard web UI: plain words match title and description, and filters such as `status:open`,
`assignee:<username>`, `title:`, `description:`, `comment:`, `tag:`, `column:` or `created:>=2026-01-01`
can be combined. The API has no global search, so every active project is searched (8 in parallel; with a
personal token every active project the user is a member of);
`-p/--project` restricts the search to projects given by id or by a case-insensitive part of the name.

```bash
kanboard-task -c config.yml -s 'invoice status:open'              # all active projects, max 50 hits
kanboard-task -c config.yml -s 'comment:"deploy"' -p webshop -p 12 # only these projects
kanboard-task -c config.yml -s 'export' --limit 0 --include-inactive
```

`total` counts every hit, `truncated` tells whether the list was cut to `--limit` (default 50). A project
whose search failed is listed in `warnings`; the others are still printed. Dump a hit by id for the full
ticket.

### Task colors

`--colors` prints the colors a task can have: `default` (the color of a task created without one) and
`colors [{id, name, background, border}]` in the order of the web UI's color menu. A task's `color` field
is one of these ids. The names are translated to the server's language. `background` and `border` (CSS
colors) are known only for Kanboard's built-in colors, not for colors that a plugin adds. In searches,
`color:<id>` also works only for the built-in colors.

```bash
kanboard-task -c config.yml --colors
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
| 3    | Kanboard API error, or a change was not made         |

## kanboard-mcp: MCP server for AI agents

`kanboard-mcp` is an [MCP](https://modelcontextprotocol.io/) server that gives AI agents access to Kanboard,
read-only unless it is started with `--writable`. It returns the same data as `kanboard-task`, but as MCP tools. It speaks MCP over stdin/stdout,
so an MCP client starts it as a subprocess. It uses the same configuration file as `kanboard-task` (only
`Kanboard.ApiUrl`, `Kanboard.Username` and `Kanboard.Password`), and the same `KANBOARD_TASK_CONFIG`
environment variable.

To use it from Claude Code:

```bash
claude mcp add kanboard /path/to/kanboard-mcp /path/to/config.yml
```

On Windows, in PowerShell (`-s user` makes it available in every project):

```powershell
claude mcp add kanboard -s user "$env:USERPROFILE\.local\bin\kanboard-mcp.exe" "C:\path\to\config.yml"
```

The config file can also be given with `-c`, or in the `KANBOARD_TASK_CONFIG` environment variable. Options of
`kanboard-mcp` itself, such as `-o DIR`, need a `--` before the command, otherwise `claude` takes them as its
own and stops with `error: unknown option`:

```bash
claude mcp add kanboard -- /path/to/kanboard-mcp -o /path/to/downloads /path/to/config.yml
```

Type that `--` as two plain hyphens. Word, Outlook, Teams and the like turn a copied `--` into a dash (`–`),
which `claude` does not recognize. In cmd.exe write `--` or `"--"`, never `'--'`. If `claude` was installed
with npm (a `claude.ps1` script), PowerShell drops a bare `--`, so write `'--'` there.

If `/mcp` shows `CONNECTION_CLOSED`, start `claude --debug` and look for the server's stderr in the debug log.
On Windows, `'-' is not recognized as an internal or external command` means that the `"command"` of the
`kanboard` entry in `~/.claude.json` is a dash (cmd.exe shows `–` as `-`) instead of the path of
`kanboard-mcp`; `'--' is not recognized` means it is `--`. Remove the entry (`claude mcp remove kanboard`, with
the same `-s` as when it was added) and add it again. Running the command and arguments of the entry in a
terminal shows any other startup error; if it just waits for input, the server itself is fine.

For an agent that works on behalf of one person, that person's personal access token is the better fit
(see the personal access token notes in the `kanboard-task` section): the agent then sees exactly what that
person can see, and
`assignee:me` works in searches.

| Tool                   | Arguments                                                     | Returns                                                  |
|------------------------|---------------------------------------------------------------|----------------------------------------------------------|
| `get_task`             | `tasks` (ids, `#KB123` or URLs), `fields`, `max_comments`     | JSON array, one `kanboard-task` document per task        |
| `search_tasks`         | `query`, `projects`, `include_inactive`, `limit` (default 50) | the `kanboard-task --search` JSON object                 |
| `get_attachment`       | `file_id`                                                     | an image (png, jpeg, gif, webp), or text (first 100 KiB) |
| `download_attachments` | `tasks` (every attachment of these tasks), `file_ids`         | the local paths of the saved files, and what was skipped |
| `list_task_colors`     | none                                                          | the `kanboard-task --colors` JSON object                 |

* `fields` limits a task to the listed top-level fields (`id` and `warnings` are always included). The API
  calls behind the other fields are skipped, so this is also faster. `max_comments` keeps only the newest
  N comments, and `comments_omitted` tells how many older ones were left out. Both help keep large tickets
  from filling the agent's context.
* A task that cannot be loaded is returned as `{"id": N, "error": "..."}` among the others; if every
  requested task fails, the call is a tool error. Errors carry a message the agent can act on.
* Screenshots pasted into comments are listed in the comment's `attachment_ids`; `get_attachment` returns
  them as images, so the agent can look at them. Files larger than `--max-attachment-bytes` (default 5 MiB)
  and other file types are refused.
* `download_attachments` saves attachments of any type (docx, pdf, xlsx, zip, ...) as
  `DIR/<task id>/<file id>-<name>`, the same layout as `kanboard-task -o`, and returns for each file its
  `local_path`, `name`, `size_bytes` and `mime_type`, plus a `skipped` list with the reason (too large, not
  found, no access). The agent then reads or copies the files with its own tools. `DIR` is chosen when the
  server starts, never by the agent: `-o`/`--download-dir`, by default `kanboard-mcp\attachments` under
  `%LocalAppData%` on Windows, `~/Library/Caches/kanboard-mcp/attachments` on macOS and
  `~/.cache/kanboard-mcp/attachments` on Linux. Attachment names are made
  safe for Windows and Unix file systems, and the files are written so that neither a name nor a symlink
  or junction inside `DIR` can lead outside it. Files larger than `--max-download-bytes` (default 20 MiB)
  are skipped. Without `--writable` this is the only tool that writes anything, and it writes nothing to
  Kanboard.
* `kanboard-mcp --info` describes the tools and lists the field names.

## Changing tasks (`--writable`)

Started with `-w`/`--writable`, `kanboard-mcp` offers more tools and `kanboard-task` accepts commands that
change Kanboard. Without the option those tools do not exist and the commands are refused. Nothing deletes
a task, a comment or an attachment.

```bash
claude mcp add kanboard -- /path/to/kanboard-mcp --writable --signature Claude /path/to/config.yml
```

| Tool (`kanboard-mcp`)  | Command (`kanboard-task`)                      | What it does                                         |
|------------------------|------------------------------------------------|------------------------------------------------------|
| `create_task`          | `create-task PROJECT TITLE [--description ...]` | new task; column, swimlane, category, owner, tags, color |
| `add_comment`          | `comment TASK --text ...`                      | comment on a task                                    |
| `update_comment`       | `edit-comment COMMENT_ID --text ...`           | replace the text of a comment written by the agent   |
| `upload_files`         | `upload TASK FILE... [--text ...]`             | attach files, with one comment that shows them       |
| `add_task_link`        | `link TASK RELATION OTHER_TASK`                | e.g. `link 12 blocks 34`                             |
| `remove_task_link`     | `unlink TASK LINK_ID`                          | remove a link between tasks                          |
| `add_external_link`    | `add-url TASK URL [--title ...]`               | attach a URL                                         |
| `remove_external_link` | `remove-url TASK LINK_ID`                      | remove a URL                                         |
| `assign_task`          | `assign TASK [--owner] [--group] [--add-other-assignee] ...` | assignee, group, other assignees  |
| `set_task_color`       | `color TASK COLOR`                             | change the color of a task                           |
| `move_task`            | `move TASK [--column] [--swimlane] [--position]` | another column, swimlane or position              |
| `close_task`, `open_task` | `close TASK`, `open TASK`                   | close or reopen                                      |

* **Whose name.** Changes are made as the Kanboard user of the config file. With a personal access token
  that is the token's owner. With the `jsonrpc` application token, comments are written as
  `Kanboard.UserId`, which must then be set, and that user becomes a new task's creator if they are a
  member of the project.
* **Signature.** With `--signature Claude`, a description or comment written by the agent ends with a
  horizontal rule and *created by Claude*; an edited comment says *created by Claude, modified by Claude*.
  Readers can then tell the agent's text from the account owner's, even though Kanboard shows the same
  author for both.
* **Editing comments** is limited to comments of the same Kanboard user that carry a signature footer, so a
  comment someone wrote by hand is never changed. Without `--signature` the two cannot be told apart, so
  editing is refused.
* **Uploads** read files only below the upload directory: `--upload-dir`, by default `kanboard-mcp/uploads`
  in the user cache directory for `kanboard-mcp` (created at start, and named in the tool description) and
  the current directory for `kanboard-task`. Paths outside it, also through a symlink, are refused, so a
  ticket that tells the agent to "upload ~/.ssh/id_rsa" gets nowhere; the agent has to copy a file there
  first, which the user sees. Files larger than `--max-upload-bytes` (default 10 MiB) are refused; the PHP
  limits of the Kanboard server (`post_max_size`) may be lower. Every upload adds one comment: the given
  text, then the files (images inline, other files as download links), then the signature.
* **Names.** Projects, columns, swimlanes, categories and users can be given by id or by name (exact, or a
  part of exactly one name, case-insensitive); a name that does not match is refused with the list of
  valid ones. The owner can also be `me`. Relations are Kanboard's link labels (`relates to`, `blocks`,
  `is blocked by`, `duplicates`, `is a child of`, `fixes`, ...); a non-admin personal token cannot list
  them, so then the default labels are used and the created link is checked.
* **Several assignees.** With the [Group_assign](https://github.com/creecros/Group_assign) plugin, a task
  can also have an assigned group and other assignees. `create_task` (`--group`, `--other-assignee`) sets
  them on a new task, and `assign_task` (`assign`) changes the assignee, the group or the other assignees
  of an existing one, only what is given: `other_assignees` replaces the list, `add_other_assignees` and
  `remove_other_assignees` change it one by one. `assign_task` needs version 1.8.4 or newer of the plugin;
  older versions can only overwrite every field at once, which is not used. The group must be added to the
  project, and a non-administrator can give it only by id, because listing groups is admin-only.
* **Colors.** `create_task` (`--color`) and `set_task_color` (`color`) take a color id or name from
  `list_task_colors` (`--colors`), case-insensitive, also with spaces instead of underscores
  (`deep orange`); an unknown color is refused with the list of ids. `set_task_color` returns the new and
  the previous color. If an automatic action of the project changes the color again (such as *assign a
  color to a category*), the result shows the color the task ended up with, and a warning.
* `kanboard-task` reads a long text from a file or stdin with `--text-file FILE` (`-` = stdin), and
  `--description-file` for `create-task`. It prints the result as JSON.

```bash
kanboard-task -c config.yml -w --signature Claude comment 13670 --text 'Fixed in !123.'
kanboard-task -c config.yml -w --signature Claude create-task webshop 'Export fails' --column Ready --tag bug \
  --description-file notes.md
kanboard-task -c config.yml -w --signature Claude upload 13670 screenshot.png --text 'After the fix:'
kanboard-task -c config.yml -w move 13670 --column Done
kanboard-task -c config.yml -w color 13670 red
```

## Development

```bash
go build ./...      # compile
go vet ./...        # static checks
go test ./...       # unit tests
```

All binaries take their `--version` information from `-ldflags -X` values set by `scripts/build.sh` and by
the GitHub Actions workflow.

## License

Apache License 2.0, see [LICENSE](LICENSE).
