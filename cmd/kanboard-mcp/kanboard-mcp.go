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
	ConfigFile         string `short:"c" long:"config" description:"Config file path (only the Kanboard section is used)" env:"KANBOARD_TASK_CONFIG"`
	MaxAttachmentBytes int64  `long:"max-attachment-bytes" description:"get_attachment refuses larger files (0 = no limit)" default:"5242880"`
	Verbose            bool   `short:"v" long:"verbose" description:"Verbose loglevel (logs go to stderr)"`
	Debug              bool   `short:"d" long:"debug" description:"Debug loglevel"`
	ShowVersion        bool   `long:"version" description:"Show version information and exit"`
	ShowInfo           bool   `short:"i" long:"info" description:"Describe the tools and exit"`
}

func main() {
	var o opts
	parser := flags.NewParser(&o, flags.Default)
	parser.Usage = "-c config.yml [OPTIONS]"
	if _, err := parser.Parse(); err != nil {
		var fe *flags.Error
		if errors.As(err, &fe) && fe.Type == flags.ErrHelp {
			os.Exit(exitOk)
		}
		os.Exit(exitUsage)
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
	server := mcpserver.New(*cfg, mcpserver.Options{
		Version:            version.LocalVersion(),
		MaxAttachmentBytes: o.MaxAttachmentBytes,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("serving MCP on stdio", "kanboard", cfg.ApiUrl, "user", cfg.Username)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitError)
	}
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
  kanboard-mcp -c config.yml [--max-attachment-bytes N] [-v]

  Speaks MCP over stdin/stdout, so it is started by an MCP client. For Claude Code:
    claude mcp add kanboard -- kanboard-mcp -c /path/to/config.yml
  The config file is the same YAML as for kanboard-task; only Kanboard.ApiUrl,
  Kanboard.Username and Kanboard.Password are read. The path can also be given
  via the KANBOARD_TASK_CONFIG environment variable. Logs go to stderr.

AUTHENTICATION
  As for kanboard-task (see kanboard-task --info): Username "jsonrpc" with the
  application API token reads every project; a login name with a personal access
  token reads with that user's permissions. For an agent that works on behalf of
  one person, that person's personal token is the better fit.

TOOLS (all read-only)
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

  Errors (task not found, no access, bad arguments) are returned as tool errors
  with a message, so the agent can see what went wrong.
`
