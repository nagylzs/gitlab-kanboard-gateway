// kanboard-task: helper that dumps everything about a Kanboard task as JSON.
//
// Intended to be called by AI agents (or scripts) that need to load the content
// of a ticket: description, comments, attachments, subtasks, links, and the
// resolved names of project / column / swimlane / category / users.
//
// With --search it instead runs a Kanboard search query in every (or the selected)
// project and prints a short summary of each matching task.
//
// With --writable its commands (create-task, comment, upload, link, move, close, ...)
// change Kanboard; without it nothing on the Kanboard server is ever modified.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jessevdk/go-flags"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskdump"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskedit"
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
	Writable         bool     `short:"w" long:"writable" description:"Allow the commands that change Kanboard (see --info)"`
	Signature        string   `long:"signature" description:"Sign descriptions and comments written by the commands with \"created by TEXT\" (default: no signature)" value-name:"TEXT"`
	UploadDir        string   `long:"upload-dir" description:"upload reads files only below DIR (default: the current directory)" value-name:"DIR"`
	MaxUploadBytes   int64    `long:"max-upload-bytes" description:"upload refuses larger files (0 = no limit)" default:"10485760"`

	CreateTask  createTaskCmd  `command:"create-task" description:"Create a task (needs --writable)"`
	Comment     commentCmd     `command:"comment" description:"Comment on a task (needs --writable)"`
	EditComment editCommentCmd `command:"edit-comment" description:"Replace the text of a signed comment of this user (needs --writable)"`
	Upload      uploadCmd      `command:"upload" description:"Attach files to a task, with a comment (needs --writable)"`
	Link        linkCmd        `command:"link" description:"Link two tasks: TASK RELATION OTHER_TASK (needs --writable)"`
	Unlink      unlinkCmd      `command:"unlink" description:"Remove an internal link of a task (needs --writable)"`
	AddUrl      addUrlCmd      `command:"add-url" description:"Attach a URL to a task (needs --writable)"`
	RemoveUrl   unlinkCmd      `command:"remove-url" description:"Remove an external link of a task (needs --writable)"`
	Move        moveCmd        `command:"move" description:"Move a task to another column, swimlane or position (needs --writable)"`
	Close       taskCmd        `command:"close" description:"Close a task (needs --writable)"`
	Open        taskCmd        `command:"open" description:"Reopen a task (needs --writable)"`
}

// textOpts is the text of a comment or description: given directly or read from a
// file (- = stdin).
type textOpts struct {
	Text     string `long:"text" description:"The text (markdown)" value-name:"TEXT"`
	TextFile string `long:"text-file" description:"Read the text from FILE (- = stdin)" value-name:"FILE"`
}

func (t textOpts) read() (string, error) {
	if t.TextFile == "" {
		return t.Text, nil
	}
	if t.Text != "" {
		return "", errors.New("give --text or --text-file, not both")
	}
	var data []byte
	var err error
	if t.TextFile == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(t.TextFile)
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type createTaskCmd struct {
	Description     string   `long:"description" description:"Task description (markdown)" value-name:"TEXT"`
	DescriptionFile string   `long:"description-file" description:"Read the description from FILE (- = stdin)" value-name:"FILE"`
	Column          string   `long:"column" description:"Column id or name (default: the first column)"`
	Swimlane        string   `long:"swimlane" description:"Swimlane id or name (default: the first one)"`
	Category        string   `long:"category" description:"Category id or name"`
	Owner           string   `long:"owner" description:"Assignee: me, user id, name or username"`
	Tags            []string `long:"tag" description:"Tag; repeatable"`
	Args            struct {
		Project string `positional-arg-name:"PROJECT" description:"Project id or name"`
		Title   string `positional-arg-name:"TITLE"`
	} `positional-args:"yes" required:"yes"`
}

type commentCmd struct {
	textOpts
	Args struct {
		Task string `positional-arg-name:"TASK"`
	} `positional-args:"yes" required:"yes"`
}

type editCommentCmd struct {
	textOpts
	Args struct {
		CommentId int `positional-arg-name:"COMMENT_ID"`
	} `positional-args:"yes" required:"yes"`
}

type uploadCmd struct {
	textOpts
	Args struct {
		Task  string   `positional-arg-name:"TASK"`
		Files []string `positional-arg-name:"FILE" required:"1"`
	} `positional-args:"yes" required:"yes"`
}

type linkCmd struct {
	Args struct {
		Task      string `positional-arg-name:"TASK"`
		Relation  string `positional-arg-name:"RELATION" description:"e.g. blocks, \"is blocked by\", \"relates to\""`
		OtherTask string `positional-arg-name:"OTHER_TASK"`
	} `positional-args:"yes" required:"yes"`
}

type unlinkCmd struct {
	Args struct {
		Task   string `positional-arg-name:"TASK"`
		LinkId int    `positional-arg-name:"LINK_ID"`
	} `positional-args:"yes" required:"yes"`
}

type addUrlCmd struct {
	Title string `long:"title" description:"Link title (default: the page title)"`
	Args  struct {
		Task string `positional-arg-name:"TASK"`
		Url  string `positional-arg-name:"URL"`
	} `positional-args:"yes" required:"yes"`
}

type moveCmd struct {
	Column   string `long:"column" description:"Column id or name (default: the current one)"`
	Swimlane string `long:"swimlane" description:"Swimlane id or name (default: the current one)"`
	Position int    `long:"position" description:"Position in the column, 1 = top (default: the bottom)"`
	Args     struct {
		Task string `positional-arg-name:"TASK"`
	} `positional-args:"yes" required:"yes"`
}

type taskCmd struct {
	Args struct {
		Task string `positional-arg-name:"TASK"`
	} `positional-args:"yes" required:"yes"`
}

func main() {
	var o opts
	parser := flags.NewParser(&o, flags.Default)
	parser.Usage = "-c config.yml [OPTIONS] TASK... | -c config.yml --writable [OPTIONS] COMMAND ..."
	// Without a command the arguments are TASKs (a top-level positional argument
	// would take a command name as a TASK).
	parser.SubcommandsOptional = true
	tasks, err := parser.Parse()
	if err != nil {
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

	if parser.Active != nil {
		if len(tasks) > 0 || o.Search != "" {
			fmt.Fprintf(os.Stderr, "error: TASK arguments and --search cannot be combined with the %v command\n", parser.Active.Name)
			os.Exit(exitUsage)
		}
		os.Exit(runCommand(o, parser.Active.Name))
	}
	if o.Signature != "" || o.UploadDir != "" {
		fmt.Fprintln(os.Stderr, "error: --signature and --upload-dir are only used by the commands that change Kanboard")
		os.Exit(exitUsage)
	}

	if o.Search != "" {
		if len(tasks) > 0 {
			fmt.Fprintln(os.Stderr, "error: TASK arguments cannot be combined with --search")
			os.Exit(exitUsage)
		}
		os.Exit(search(o))
	}
	if len(o.Projects) > 0 || o.IncludeInactive {
		fmt.Fprintln(os.Stderr, "error: --project and --include-inactive require --search")
		os.Exit(exitUsage)
	}

	if len(tasks) == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one TASK argument is required (see --help)")
		os.Exit(exitUsage)
	}
	ids := make([]int, 0, len(tasks))
	for _, ref := range tasks {
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

// runCommand runs one of the commands that change Kanboard and returns the exit code.
func runCommand(o opts, name string) int {
	if !o.Writable {
		fmt.Fprintf(os.Stderr, "error: the %v command changes Kanboard; it needs --writable\n", name)
		return exitUsage
	}
	if err := taskedit.ValidateSignature(o.Signature); err != nil {
		fmt.Fprintf(os.Stderr, "error: --signature: %v\n", err)
		return exitUsage
	}
	uploadDir := o.UploadDir
	if uploadDir == "" {
		uploadDir = "."
	}
	uploadDir, err := filepath.Abs(uploadDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: --upload-dir: %v\n", err)
		return exitUsage
	}
	cfg, err := config.LoadKanboardConfig(o.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitUsage
	}
	ed := taskedit.NewEditor(*cfg, taskedit.Options{Signature: o.Signature, UploadDir: uploadDir, MaxUploadBytes: o.MaxUploadBytes})

	var res any
	switch name {
	case "create-task":
		c := o.CreateTask
		desc, terr := textOpts{Text: c.Description, TextFile: c.DescriptionFile}.read()
		if terr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", terr)
			return exitUsage
		}
		res, err = ed.CreateTask(taskedit.CreateTaskInput{
			Project: c.Args.Project, Title: c.Args.Title, Description: desc,
			Column: c.Column, Swimlane: c.Swimlane, Category: c.Category, Owner: c.Owner, Tags: c.Tags,
		})
	case "comment", "edit-comment", "upload":
		var t textOpts
		switch name {
		case "comment":
			t = o.Comment.textOpts
		case "edit-comment":
			t = o.EditComment.textOpts
		default:
			t = o.Upload.textOpts
		}
		text, terr := t.read()
		if terr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", terr)
			return exitUsage
		}
		switch name {
		case "comment":
			res, err = ed.AddComment(o.Comment.Args.Task, text)
		case "edit-comment":
			res, err = ed.UpdateComment(o.EditComment.Args.CommentId, text)
		default:
			res, err = ed.UploadFiles(o.Upload.Args.Task, o.Upload.Args.Files, text)
		}
	case "link":
		res, err = ed.AddTaskLink(o.Link.Args.Task, o.Link.Args.OtherTask, o.Link.Args.Relation)
	case "unlink":
		res, err = ed.RemoveTaskLink(o.Unlink.Args.Task, o.Unlink.Args.LinkId)
	case "add-url":
		res, err = ed.AddExternalLink(o.AddUrl.Args.Task, o.AddUrl.Args.Url, o.AddUrl.Title)
	case "remove-url":
		res, err = ed.RemoveExternalLink(o.RemoveUrl.Args.Task, o.RemoveUrl.Args.LinkId)
	case "move":
		res, err = ed.MoveTask(o.Move.Args.Task, o.Move.Column, o.Move.Swimlane, o.Move.Position)
	case "close":
		res, err = ed.CloseTask(o.Close.Args.Task)
	case "open":
		res, err = ed.OpenTask(o.Open.Args.Task)
	default:
		panic("unknown command " + name)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		var nf taskdump.ErrNotFound
		if errors.As(err, &nf) {
			return exitNotFound
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

const infoText = `kanboard-task - dump a Kanboard task as JSON (read-only without --writable)

USAGE
  kanboard-task -c config.yml [-o DIR] TASK [TASK...]
  kanboard-task -c config.yml -s QUERY [-p PROJECT]... [--limit N]
  kanboard-task -c config.yml --writable [--signature TEXT] COMMAND ...   (see COMMANDS)

  TASK may be a number (123), a hash reference (#123, #KB123), or a task URL.
  The config file is the same YAML as used by gitlab-kanboard-gateway; only
  Kanboard.ApiUrl, Kanboard.Username and Kanboard.Password are read.
  The path can also be given via the KANBOARD_TASK_CONFIG environment variable.
  The commands also read Kanboard.UserId (see COMMANDS).

AUTHENTICATION
  Username "jsonrpc" with the application API token (Settings > API) can read
  every project. Any other Username logs in as that Kanboard user, with a personal
  access token (My profile > API) as Password, and gets that user's permissions:
  - only projects the user is a member of, even for Kanboard admins: other tasks
    fail with a 403 API error (exit 3), --search covers only the user's projects;
  - for a non-admin user, owner and creator have only {id,name} (no username or
    email), and only if they are members of the task's project;
  - comments restricted to managers or admins are left out for users below
    that role.

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
  owner {id,username,name,email} | null      the assignee (see AUTHENTICATION)
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
  object. With a personal token only the user's own projects are searched.
  QUERY uses the web UI search syntax: plain words match the title and
  description; filters such as status:open, assignee:<username>, creator:...,
  title:"...", description:..., comment:..., tag:..., column:..., category:...,
  created:>=2026-01-01 can be combined ("me", as in assignee:me, works only with a
  personal token). Without a status: filter both open and closed tasks are returned.

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

COMMANDS (change Kanboard; only with -w/--writable)
  create-task PROJECT TITLE [--description TEXT | --description-file FILE]
              [--column C] [--swimlane S] [--category C] [--owner U] [--tag T]...
  comment TASK (--text TEXT | --text-file FILE)
  edit-comment COMMENT_ID (--text TEXT | --text-file FILE)
  upload TASK FILE... [--text TEXT | --text-file FILE]
  link TASK RELATION OTHER_TASK     e.g. link 12 blocks 34, link 12 "is blocked by" 34
  unlink TASK LINK_ID               LINK_ID from the task's links
  add-url TASK URL [--title TITLE]
  remove-url TASK LINK_ID           LINK_ID from the task's external_links
  move TASK [--column C] [--swimlane S] [--position N]   (1 = top, default: bottom)
  close TASK, open TASK

  Each prints a JSON object about the result (ids, the stored text, the task's
  column and status). Nothing deletes a task, a comment or an attachment.
  --text-file - reads the text from stdin.

  Projects, columns, swimlanes, categories and users are given by id or name
  (exact, or a part of exactly one name, case-insensitive); a wrong name is
  refused with the list of valid ones. --owner also accepts "me" and usernames.
  RELATION is a Kanboard link label: relates to, blocks, is blocked by,
  duplicates, is duplicated by, is a child of, is a parent of, fixes, ...

  Changes are made as the config's Kanboard user (the personal token's owner).
  With the jsonrpc token, comments are written as Kanboard.UserId, which must
  be set, and that user is a new task's creator if a member of the project.

  --signature TEXT ends a description or comment written by a command with a
  horizontal rule and "*created by TEXT*"; edit-comment makes it "*created by
  TEXT, modified by TEXT*". edit-comment changes only comments of the same user
  that carry such a footer, so never one written by hand, and needs --signature.

  upload reads files only below --upload-dir (default: the current directory);
  paths outside it, also through a symlink, are refused. Files larger than
  --max-upload-bytes (default 10 MiB) are refused. It adds one comment: the
  text, then the files (images shown inline, other files as download links).

  Exit codes: 0 ok, 1 usage error (also a command without --writable), 2 task
  not found, 3 API error or the change was not made (the message says why).

EXAMPLES
  kanboard-task -c config.yml 13670
  kanboard-task -c config.yml -o ./attachments 13670 | jq .attachments
  kanboard-task -c config.yml --compact 1 2 3
  kanboard-task -c config.yml -s 'invoice status:open'
  kanboard-task -c config.yml -s 'dashboard' -p webshop -p 12 --limit 0
  kanboard-task -c config.yml -w --signature Claude comment 13670 --text 'Fixed in !123.'
  kanboard-task -c config.yml -w --signature Claude upload 13670 shot.png --text 'After:'
  kanboard-task -c config.yml -w move 13670 --column Done
`
