// kanboard-task: read-only helper that dumps everything about a Kanboard task as JSON.
//
// Intended to be called by AI agents (or scripts) that need to load the content
// of a ticket: description, comments, attachments, subtasks, links, and the
// resolved names of project / column / swimlane / category / users.
//
// With --search it instead runs a Kanboard search query in every (or the selected)
// project and prints a short summary of each matching task.
//
// It never modifies anything on the Kanboard server.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskdump"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/version"
)

const (
	exitOk       = 0
	exitUsage    = 1
	exitNotFound = 2
	exitApiError = 3
)

type opts struct {
	ConfigFile       string   `short:"c" long:"config" description:"Config file path (only the Kanboard section is used)" env:"KANBOARD_TASK_CONFIG"`
	DownloadDir      string   `short:"o" long:"download-dir" description:"Download attachments into DIR/<task-id>/ and report their local paths" value-name:"DIR"`
	MaxDownloadBytes int64    `long:"max-download-bytes" description:"Skip attachments larger than this (0 = no limit)" default:"20971520"`
	Compact          bool     `long:"compact" description:"Print single-line JSON instead of indented"`
	Verbose          bool     `short:"v" long:"verbose" description:"Verbose loglevel (logs go to stderr)"`
	Debug            bool     `short:"d" long:"debug" description:"Debug loglevel"`
	ShowVersion      bool     `long:"version" description:"Show version information and exit"`
	ShowInfo         bool     `short:"i" long:"info" description:"Describe the output format and exit"`
	Search           string   `short:"s" long:"search" description:"Search tasks with a Kanboard query (web UI search syntax) instead of dumping TASKs" value-name:"QUERY"`
	Projects         []string `short:"p" long:"project" description:"With --search: only this project (id, or part of the name); repeatable" value-name:"PROJECT"`
	IncludeInactive  bool     `long:"include-inactive" description:"With --search: also search inactive projects"`
	Limit            int      `long:"limit" description:"With --search: print at most this many hits (0 = no limit)" default:"50"`
	Args             struct {
		Tasks []string `positional-arg-name:"TASK" description:"Task number, #KB123, or task URL (one or more)"`
	} `positional-args:"yes"`
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
		fmt.Print(infoText)
		os.Exit(exitOk)
	}

	level := new(slog.LevelVar)
	level.Set(slog.LevelWarn)
	if o.Debug {
		level.Set(slog.LevelDebug)
	} else if o.Verbose {
		level.Set(slog.LevelInfo)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if o.Search != "" {
		if len(o.Args.Tasks) > 0 {
			fmt.Fprintln(os.Stderr, "error: TASK arguments cannot be combined with --search")
			os.Exit(exitUsage)
		}
		os.Exit(search(o))
	}
	if len(o.Projects) > 0 || o.IncludeInactive {
		fmt.Fprintln(os.Stderr, "error: --project and --include-inactive require --search")
		os.Exit(exitUsage)
	}

	if len(o.Args.Tasks) == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one TASK argument is required (see --help)")
		os.Exit(exitUsage)
	}
	ids := make([]int, 0, len(o.Args.Tasks))
	for _, ref := range o.Args.Tasks {
		id, err := taskdump.ParseTaskId(ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(exitUsage)
		}
		ids = append(ids, id)
	}

	cfg, err := config.LoadKanboardConfig(o.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitUsage)
	}

	dumper := taskdump.NewDumper(*cfg, taskdump.Options{
		DownloadDir:      o.DownloadDir,
		MaxDownloadBytes: o.MaxDownloadBytes,
	})

	docs := make([]*taskdump.TaskDocument, 0, len(ids))
	exit := exitOk
	for _, id := range ids {
		doc, err := dumper.Dump(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			var nf taskdump.ErrNotFound
			if errors.As(err, &nf) {
				exit = max(exit, exitNotFound)
			} else {
				exit = max(exit, exitApiError)
			}
			continue
		}
		docs = append(docs, doc)
	}

	if len(docs) > 0 {
		var out interface{} = docs
		if len(ids) == 1 {
			out = docs[0]
		}
		if err := printJson(out, o.Compact); err != nil {
			os.Exit(exitApiError)
		}
	}
	os.Exit(exit)
}

func search(o opts) int {
	cfg, err := config.LoadKanboardConfig(o.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitUsage
	}
	res, err := taskdump.Search(*cfg, taskdump.SearchOptions{
		Query:           o.Search,
		Projects:        o.Projects,
		IncludeInactive: o.IncludeInactive,
		Limit:           o.Limit,
		Workers:         8,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		var np taskdump.ErrNoProject
		if errors.As(err, &np) {
			return exitUsage
		}
		return exitApiError
	}
	if err := printJson(res, o.Compact); err != nil {
		return exitApiError
	}
	return exitOk
}

func printJson(v interface{}, compact bool) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if !compact {
		enc.SetIndent("", "  ")
	}
	err := enc.Encode(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot encode JSON: %v\n", err)
	}
	return err
}

const infoText = `kanboard-task - dump a Kanboard task as JSON (read-only)

USAGE
  kanboard-task -c config.yml [-o DIR] TASK [TASK...]
  kanboard-task -c config.yml -s QUERY [-p PROJECT]... [--limit N]

  TASK may be a number (123), a hash reference (#123, #KB123), or a task URL.
  The config file is the same YAML as used by gitlab-kanboard-gateway; only
  Kanboard.ApiUrl, Kanboard.Username and Kanboard.Password are read.
  The path can also be given via the KANBOARD_TASK_CONFIG environment variable.

OUTPUT
  stdout: one JSON object for a single TASK, a JSON array when several are given.
  stderr: log lines and errors (never JSON data).
  Exit codes: 0 ok, 1 usage/config error, 2 task not found, 3 Kanboard API error.
  With several TASKs, the tasks that could be loaded are still printed.

JSON FIELDS (top level)
  id, url, title                task identity; url opens the task in a browser
  description                   task body, markdown
  status                        "open" or "closed"
  project {id,name,identifier,board_url}
  column {id,name}              board column the task is in (null if unknown)
  swimlane {id,name} | null
  category {id,name} | null
  color, priority, score, reference, position
  owner {id,username,name,email} | null      the assignee
  creator {id,username,name,email} | null
  dates {created,modified,moved,started,due,completed}   RFC3339 or null
  time_tracking {spent_hours,estimated_hours}
  tags [string]
  metadata {key: value}         custom task metadata
  recurrence {...}              only present for recurring tasks
  external_task {provider,uri}  only present when the task came from an external provider
  subtasks [{id,title,status,assignee,time_tracking}]   status: todo|in_progress|done
  comments [{id,created,modified,author{id,username,name,email},visibility,content,attachment_ids?}]
                                content is markdown; ordered as returned by Kanboard (oldest first).
                                Images pasted into a comment are stored as task attachments; their
                                relative <img> tags are rewritten to ![<attachment name>](<target>)
                                where target is the file's local_path when --download-dir was given,
                                otherwise the absolute Kanboard URL. attachment_ids lists the ids of
                                the entries in "attachments" that the comment embeds.
  attachments [{id,name,size_bytes,is_image,uploaded,uploaded_by,local_path?,download_error?}]
                                local_path is set only when --download-dir was given and
                                the file was saved to DIR/<task-id>/<file-id>-<name>
  links [{id,relation,task{id,title,status,project{id,name},column,assignee,dates,time_tracking}}]
                                internal links to other Kanboard tasks; relation is e.g.
                                "relates to", "blocks", "is blocked by", "duplicates"
  external_links [{id,type,relation,title,url,created,modified,creator}]
                                URLs attached to the task (type: weblink, attachment, ...)
  warnings [string]             present only if some secondary API call failed;
                                the affected list is then empty rather than missing
  fetched_at                    RFC3339 time the data was read

SEARCH (--search QUERY)
  Runs Kanboard's searchTasks in every active project (or only in the --project
  ones, matched by id or case-insensitive part of the name) and prints one JSON
  object. QUERY uses the web UI search syntax: plain words match the title and
  description; filters such as status:open, assignee:<username>, creator:...,
  title:"...", description:..., comment:..., tag:..., column:..., category:...,
  created:>=2026-01-01 can be combined ("me" does not work: the API user is not a
  Kanboard user). Without a status: filter both open and closed tasks are returned.

  query, projects_searched
  total, truncated              total counts every hit; tasks is cut to --limit
                                (default 50, 0 = no limit) when truncated is true
  tasks [{id,url,title,status,project{id,name},column,swimlane?,category?,
          assignee{id,username,name}|null,priority,reference?,
          dates{created,modified,due,completed},time_tracking,
          counts{comments,attachments,subtasks,subtasks_done,links,external_links},
          description_excerpt}]
                                most recently modified first; description_excerpt is
                                the first 300 characters with whitespace collapsed.
                                Dump a hit by id for the full ticket.
  warnings [string]             projects whose search failed (the rest are still listed)
  fetched_at
  Exit codes: 0 ok (also with no hits), 1 usage error or no project matches
  --project, 3 API error (project list failed, or the search failed everywhere).

EXAMPLES
  kanboard-task -c config.yml 13670
  kanboard-task -c config.yml -o ./attachments 13670 | jq .attachments
  kanboard-task -c config.yml --compact 1 2 3
  kanboard-task -c config.yml -s 'invoice status:open'
  kanboard-task -c config.yml -s 'dashboard' -p webshop -p 12 --limit 0
`
