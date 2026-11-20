// kanboard-mcp: MCP server (stdio) that gives AI agents access to Kanboard tasks. It
// offers the same data as kanboard-task, as MCP tools.
//
// It modifies nothing on the Kanboard server unless it is started with --writable.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jessevdk/go-flags"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/mcpserver"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskdump"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskedit"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/version"
)

const (
	exitOk    = 0
	exitUsage = 1
	exitError = 3
)

type opts struct {
	ConfigFile         string `short:"c" long:"config" description:"Config file path (only the Kanboard section is used); can also be given as the argument" env:"KANBOARD_TASK_CONFIG"`
	MaxAttachmentBytes int64  `long:"max-attachment-bytes" description:"get_attachment refuses larger files (0 = no limit)" default:"5242880"`
	DownloadDir        string `short:"o" long:"download-dir" description:"download_attachments saves files into DIR/<task-id>/ (default: kanboard-mcp/attachments in the user cache directory)" value-name:"DIR"`
	MaxDownloadBytes   int64  `long:"max-download-bytes" description:"download_attachments skips larger files (0 = no limit)" default:"20971520"`
	Writable           bool   `short:"w" long:"writable" description:"Also offer the tools that change Kanboard (create tasks, comments, uploads, links, move, close)"`
	Signature          string `long:"signature" description:"With --writable: sign descriptions and comments written by the agent with \"created by TEXT\" (default: no signature)" value-name:"TEXT"`
	UploadDir          string `long:"upload-dir" description:"With --writable: upload_files reads files only below DIR (default: kanboard-mcp/uploads in the user cache directory)" value-name:"DIR"`
	MaxUploadBytes     int64  `long:"max-upload-bytes" description:"With --writable: upload_files refuses larger files (0 = no limit)" default:"10485760"`
	Verbose            bool   `short:"v" long:"verbose" description:"Verbose loglevel (logs go to stderr)"`
	Debug              bool   `short:"d" long:"debug" description:"Debug loglevel"`
	ShowVersion        bool   `long:"version" description:"Show version information and exit"`
	ShowInfo           bool   `short:"i" long:"info" description:"Describe the tools and exit"`
}

func main() {
	var o opts
	parser := flags.NewParser(&o, flags.Default)
	parser.Usage = "[OPTIONS] config.yml"
	args, err := parser.Parse()
	if err != nil {
		var fe *flags.Error
		if errors.As(err, &fe) && fe.Type == flags.ErrHelp {
			os.Exit(exitOk)
		}
		os.Exit(exitUsage)
	}
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "error: only one config file can be given, got %q\n", args)
		os.Exit(exitUsage)
	}
	if len(args) == 1 {
		// Given as an argument, the config file needs no "-c", so "claude mcp add"
		// needs no "--" before the command (PowerShell drops a bare "--").
		o.ConfigFile = args[0]
	}
	if o.ShowVersion {
		version.PrintVersion()
		os.Exit(exitOk)
	}
	if o.ShowInfo {
		fmt.Printf(infoText, wrap(strings.Join(taskdump.TaskFields, ", "), 63, strings.Repeat(" ", 17)), mcpserver.MaxTextBytes/1024)
		os.Exit(exitOk)
	}

	// stdout carries the MCP protocol, so logs must go to stderr.
	level := new(slog.LevelVar)
	level.Set(slog.LevelWarn)
	if o.Debug {
		level.Set(slog.LevelDebug)
	} else if o.Verbose {
		level.Set(slog.LevelInfo)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	cfg, err := config.LoadKanboardConfig(o.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitUsage)
	}
	if !o.Writable && (o.Signature != "" || o.UploadDir != "") {
		fmt.Fprintln(os.Stderr, "error: --signature and --upload-dir require --writable")
		os.Exit(exitUsage)
	}
	if err := taskedit.ValidateSignature(o.Signature); err != nil {
		fmt.Fprintf(os.Stderr, "error: --signature: %v\n", err)
		os.Exit(exitUsage)
	}
	downloadDir, err := resolveCacheDir(o.DownloadDir, "attachments")
	if err != nil {
		// download_attachments reports the missing directory; the other tools still work.
		slog.Warn("no download directory", "error", err)
	}
	var uploadDir string
	if o.Writable {
		uploadDir, err = resolveCacheDir(o.UploadDir, "uploads")
		if err == nil {
			// It must exist for the agent to copy files into it.
			err = os.MkdirAll(uploadDir, 0o755)
		}
		if err != nil {
			// upload_files reports the missing directory; the other tools still work.
			slog.Warn("no upload directory", "error", err)
			uploadDir = ""
		}
	}
	server := mcpserver.New(*cfg, mcpserver.Options{
		Version:            version.LocalVersion(),
		MaxAttachmentBytes: o.MaxAttachmentBytes,
		DownloadDir:        downloadDir,
		MaxDownloadBytes:   o.MaxDownloadBytes,
		Writable:           o.Writable,
		Signature:          o.Signature,
		UploadDir:          uploadDir,
		MaxUploadBytes:     o.MaxUploadBytes,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("serving MCP on stdio", "kanboard", cfg.ApiUrl, "user", cfg.Username, "download_dir", downloadDir,
		"writable", o.Writable, "upload_dir", uploadDir)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitError)
	}
}

// resolveCacheDir returns dir as an absolute path, or by default kanboard-mcp/<name>
// in the user cache directory (%LocalAppData% on Windows, ~/Library/Caches on macOS,
// ~/.cache on Linux). The download directory is created on the first download.
func resolveCacheDir(dir, name string) (string, error) {
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(cache, "kanboard-mcp", name)
	}
	return filepath.Abs(dir)
}

// wrap breaks s at spaces into lines of at most width characters; lines after the
// first one start with indent.
func wrap(s string, width int, indent string) string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n"+indent)
}

const infoText = `kanboard-mcp - MCP server with access to Kanboard tasks (read-only by default)

USAGE
  kanboard-mcp [-c] config.yml [--max-attachment-bytes N] [-o DIR] [--max-download-bytes N] [-v]
  kanboard-mcp [-c] config.yml --writable [--signature TEXT] [--upload-dir DIR] [--max-upload-bytes N] ...

  Speaks MCP over stdin/stdout, so it is started by an MCP client. For Claude Code:
    claude mcp add kanboard /path/to/kanboard-mcp /path/to/config.yml
  The config file is the same YAML as for kanboard-task; only Kanboard.ApiUrl,
  Kanboard.Username and Kanboard.Password are read (and Kanboard.UserId, see
  AUTHENTICATION). It can be given as the
  argument, with -c, or via the KANBOARD_TASK_CONFIG environment variable.
  Options of kanboard-mcp (-o, ...) need "--" before the command, typed as two
  plain hyphens (a dash copied from a document does not work):
    claude mcp add kanboard -- /path/to/kanboard-mcp -o DIR /path/to/config.yml
    claude mcp add kanboard -- /path/to/kanboard-mcp --writable --signature Claude /path/to/config.yml
  Logs go to stderr.

AUTHENTICATION
  As for kanboard-task (see kanboard-task --info): Username "jsonrpc" with the
  application API token reads every project; a login name with a personal access
  token reads with that user's permissions. For an agent that works on behalf of
  one person, that person's personal token is the better fit.
  With --writable, changes are made in the name of that user; with the jsonrpc
  token, comments are written as the user Kanboard.UserId (required then).

TOOLS (read-only)
  get_task        tasks: [id | #KB123 | task URL, ...]   (required)
                  fields: [name, ...]   only these top-level fields
                  max_comments: N       only the newest N comments
    Returns a JSON array with one entry per task: the object kanboard-task prints
    (see kanboard-task --info), or {"id": N, "error": "..."} if it failed.
    Field names: %s
    id and warnings are always included. The API calls behind fields that were
    not asked for (comments, subtasks, links, names, ...) are skipped. With
    max_comments, comments_omitted counts the older comments left out.

  search_tasks    query: Kanboard search syntax   (required)
                  projects: [id | part of the name, ...]
                  include_inactive: bool
                  limit: N   (default 50, 0 = no limit)
    Returns the JSON object kanboard-task --search prints.

  get_attachment  file_id: N   (from attachments or a comment's attachment_ids)
    Returns an image (png, jpeg, gif, webp) as image content, or a text file
    as text, cut at %d KiB. Other file types and files larger than
    --max-attachment-bytes are refused.

  download_attachments
                  tasks: [id | #KB123 | task URL, ...]
                  file_ids: [N, ...]
    Saves attachments of any type as DIR/<task id>/<file id>-<name>, every
    attachment of the given tasks and/or the given files. Returns a JSON object:
    directory, files [{file_id, task_id, name, size_bytes, mime_type,
    local_path}] and skipped [{file_id, task_id, name, size_bytes, reason}].
    DIR is set with -o/--download-dir (default: kanboard-mcp/attachments in the
    user cache directory, i.e. %%LocalAppData%% on Windows, ~/Library/Caches on
    macOS, ~/.cache on Linux); the agent cannot write anywhere else. Names are
    made safe for Windows and Unix file systems. Files larger than
    --max-download-bytes (default 20 MiB) are skipped. An earlier copy of the
    same file is replaced. Without --writable this is the only tool that writes
    anything, and only below DIR, never to Kanboard.

TOOLS THAT CHANGE KANBOARD (only with -w/--writable)
  Without --writable these tools do not exist. None of them deletes a task, a
  comment or an attachment. Tasks, columns, users and so on can be given by id
  or name; a name that does not match is refused with the list of valid ones.

  Signature: with --signature Claude, a task description or comment written by
  the agent ends with a horizontal rule and "*created by Claude*", and an
  edited comment with "*created by Claude, modified by Claude*". So readers can
  tell the agent's text from the account owner's.

  create_task     project, title (required); description, column, swimlane,
                  category, owner (me | id | name | username), tags: [name, ...],
                  group, other_assignees: [user, ...]   (Group_assign plugin)
  assign_task     task; owner (me | user | nobody), group (id | name | none),
                  other_assignees: [user, ...] (replaces the list) or
                  add_other_assignees / remove_other_assignees: [user, ...]
    Only the given fields change. group and the other assignees need the
    Group_assign plugin, 1.8.4 or newer, on the server; a group must be added
    to the project, and a non-admin user can give it only by id.
  add_comment     task, content
  update_comment  comment_id, content   only comments of this server's Kanboard
                  user that have a signature footer, so never a comment the
                  account owner wrote by hand; without --signature it always fails
  upload_files    task, files: [path, ...], comment
    Uploads files from the upload directory (--upload-dir, default:
    kanboard-mcp/uploads in the user cache directory, created at start) and
    adds one comment: the comment text, the files (images shown inline, other
    files as download links), and the signature. Files outside the directory
    (also through a symlink) are refused, so a ticket cannot make the agent
    publish an arbitrary local file; the agent has to copy the file there first.
    Larger files than --max-upload-bytes (default 10 MiB) are refused; the PHP
    limits of the Kanboard server (post_max_size) may be lower.
  add_task_link   task, relation, other_task   e.g. "12 blocks 34"; relation is
                  one of Kanboard's link labels (relates to, blocks, is blocked
                  by, duplicates, is a child of, fixes, ...)
  remove_task_link      task, link_id   (an id from get_task's links)
  add_external_link     task, url, title (default: the page title)
  remove_external_link  task, link_id   (an id from get_task's external_links)
  move_task       task; column, swimlane, position (1 = top, default: bottom)
  close_task, open_task   task
  Each returns a short JSON object about the result (ids, the stored text).

  Errors (task not found, no access, bad arguments) are returned as tool errors
  with a message, so the agent can see what went wrong.
`
