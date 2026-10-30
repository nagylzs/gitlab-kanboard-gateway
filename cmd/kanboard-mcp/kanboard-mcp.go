// kanboard-mcp: MCP server (stdio) that gives AI agents read-only access to Kanboard
// tasks. It offers the same data as kanboard-task, as MCP tools.
//
// It never modifies anything on the Kanboard server.
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
	downloadDir, err := resolveDownloadDir(o.DownloadDir)
	if err != nil {
		// download_attachments reports the missing directory; the other tools still work.
		slog.Warn("no download directory", "error", err)
	}
	server := mcpserver.New(*cfg, mcpserver.Options{
		Version:            version.LocalVersion(),
		MaxAttachmentBytes: o.MaxAttachmentBytes,
		DownloadDir:        downloadDir,
		MaxDownloadBytes:   o.MaxDownloadBytes,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("serving MCP on stdio", "kanboard", cfg.ApiUrl, "user", cfg.Username, "download_dir", downloadDir)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitError)
	}
}

// resolveDownloadDir returns the absolute download directory: dir, or by default
// kanboard-mcp/attachments in the user cache directory (%LocalAppData% on Windows,
// ~/.cache on Linux). The directory is created on the first download.
func resolveDownloadDir(dir string) (string, error) {
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(cache, "kanboard-mcp", "attachments")
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

const infoText = `kanboard-mcp - MCP server with read-only access to Kanboard tasks

USAGE
  kanboard-mcp [-c] config.yml [--max-attachment-bytes N] [-o DIR] [--max-download-bytes N] [-v]

  Speaks MCP over stdin/stdout, so it is started by an MCP client. For Claude Code:
    claude mcp add kanboard /path/to/kanboard-mcp /path/to/config.yml
  The config file is the same YAML as for kanboard-task; only Kanboard.ApiUrl,
  Kanboard.Username and Kanboard.Password are read. It can be given as the
  argument, with -c, or via the KANBOARD_TASK_CONFIG environment variable.
  Options of kanboard-mcp (-o, ...) need "--" before the command, typed as two
  plain hyphens (a dash copied from a document does not work):
    claude mcp add kanboard -- /path/to/kanboard-mcp -o DIR /path/to/config.yml
  Logs go to stderr.

AUTHENTICATION
  As for kanboard-task (see kanboard-task --info): Username "jsonrpc" with the
  application API token reads every project; a login name with a personal access
  token reads with that user's permissions. For an agent that works on behalf of
  one person, that person's personal token is the better fit.

TOOLS (read-only on Kanboard)
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
    user cache directory, i.e. %%LocalAppData%% on Windows, ~/.cache on Linux);
    the agent cannot write anywhere else. Names are made safe for Windows and
    Unix file systems. Files larger than --max-download-bytes (default 20 MiB)
    are skipped. An earlier copy of the same file is replaced. This is the only
    tool that writes anything, and only below DIR, never to Kanboard.

  Errors (task not found, no access, bad arguments) are returned as tool errors
  with a message, so the agent can see what went wrong.
`
