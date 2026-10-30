// Package mcpserver exposes the read-only Kanboard lookups of taskdump as MCP tools.
// No tool ever calls a Kanboard procedure that modifies data. The only tool that
// writes anything is download_attachments, and only below Options.DownloadDir.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskdump"
)

type Options struct {
	// Reported to the client in the initialize handshake.
	Version string
	// get_attachment refuses files larger than this (0 = no limit).
	MaxAttachmentBytes int64
	// download_attachments saves files into <DownloadDir>/<taskId>/. The agent cannot
	// choose another place. Empty disables the tool.
	DownloadDir string
	// download_attachments skips files larger than this (0 = no limit).
	MaxDownloadBytes int64
}

// MaxTextBytes is where get_attachment cuts text files.
const MaxTextBytes = 100 * 1024

// Image types that MCP clients can be expected to show.
var imageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

const instructions = `Read-only access to a Kanboard server; nothing is ever modified.
- get_task: everything about one or more tasks as JSON. Use fields and max_comments to keep big tickets small.
- search_tasks: find tasks with the Kanboard search syntax; returns short summaries, use get_task for the full ticket.
- get_attachment: shows an attachment, e.g. a screenshot embedded in a comment (see the comment's attachment_ids).
- download_attachments: saves attachments of any type (docx, pdf, xlsx, zip, ...) into local files and returns their paths.`

// New builds the server. Each tool call creates its own taskdump.Dumper, so calls
// may run concurrently.
func New(cfg config.KanboardConfig, opts Options) *mcp.Server {
	s := mcp.NewServer(
		&mcp.Implementation{Name: "kanboard-mcp", Version: opts.Version},
		&mcp.ServerOptions{Instructions: instructions, Logger: slog.Default()},
	)
	t := &tools{cfg: cfg, opts: opts}
	mcp.AddTool(s, getTaskTool(), t.getTask)
	mcp.AddTool(s, searchTasksTool(), t.searchTasks)
	mcp.AddTool(s, getAttachmentTool(), t.getAttachment)
	mcp.AddTool(s, downloadAttachmentsTool(), t.downloadAttachments)
	return s
}

type tools struct {
	cfg  config.KanboardConfig
	opts Options
}

var readOnly = &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}

// --- get_task ---

type getTaskInput struct {
	Tasks       []string `json:"tasks" jsonschema:"Task ids (123), #KB123 references or task URLs"`
	Fields      []string `json:"fields,omitempty" jsonschema:"Top-level fields to return (default: all). id and warnings are always included. Fields not asked for are not fetched, which also makes the call faster."`
	MaxComments int      `json:"max_comments,omitempty" jsonschema:"Return only the newest N comments (0 = all); comments_omitted tells how many older ones were left out"`
}

func getTaskTool() *mcp.Tool {
	schema := mustSchema[getTaskInput]()
	tasks := schema.Properties["tasks"]
	tasks.Types, tasks.Type = nil, "array" // the inferred schema also allows null
	tasks.MinItems = ptr(1)
	fields := make([]any, len(taskdump.TaskFields))
	for i, f := range taskdump.TaskFields {
		fields[i] = f
	}
	schema.Properties["fields"].Items.Enum = fields
	schema.Properties["max_comments"].Minimum = ptr(0.0)
	return &mcp.Tool{
		Name:  "get_task",
		Title: "Get Kanboard tasks",
		Description: `Returns everything about one or more Kanboard tasks as a JSON array, one object per requested task, in the requested order: ` +
			`title, description (markdown), status, project, column, swimlane, category, owner (the assignee), creator, dates (RFC 3339), ` +
			`time_tracking, tags, metadata, subtasks, comments (markdown, oldest first), attachments, links to other tasks and external_links. ` +
			`Ids are resolved to names. A task that could not be loaded is returned as {"id": N, "error": "..."}. ` +
			`A "warnings" list means some part of the task could not be loaded; that part is then empty. ` +
			`Images pasted into a comment are listed in its attachment_ids; view them with get_attachment.`,
		InputSchema: schema,
		Annotations: readOnly,
	}
}

func (t *tools) getTask(_ context.Context, _ *mcp.CallToolRequest, in getTaskInput) (*mcp.CallToolResult, any, error) {
	if err := taskdump.ValidateFields(in.Fields); err != nil {
		return nil, nil, err
	}
	ids := make([]int, 0, len(in.Tasks))
	for _, ref := range in.Tasks {
		id, err := taskdump.ParseTaskId(ref)
		if err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
	}

	dumper := taskdump.NewDumper(t.cfg, taskdump.Options{Fields: in.Fields, MaxComments: in.MaxComments})
	entries := make([]json.RawMessage, 0, len(ids))
	var errs []error
	for _, id := range ids {
		entry, err := t.dumpOne(dumper, id, in.Fields)
		if err != nil {
			errs = append(errs, err)
			entry, _ = taskdump.MarshalJSON(struct {
				Id    int    `json:"id"`
				Error string `json:"error"`
			}{id, err.Error()})
		}
		entries = append(entries, entry)
	}
	if len(errs) == len(ids) {
		return nil, nil, errors.Join(errs...)
	}
	return textResult(entries)
}

func (t *tools) dumpOne(dumper *taskdump.Dumper, id int, fields []string) (json.RawMessage, error) {
	doc, err := dumper.Dump(id)
	if err != nil {
		return nil, accessHint(err)
	}
	return taskdump.SelectFields(doc, fields)
}

// --- search_tasks ---

type searchTasksInput struct {
	Query           string   `json:"query" jsonschema:"Kanboard search syntax, as in the web UI search box"`
	Projects        []string `json:"projects,omitempty" jsonschema:"Only these projects: ids or case-insensitive parts of the name (default: every active project)"`
	IncludeInactive bool     `json:"include_inactive,omitempty" jsonschema:"Also search inactive projects (when projects is not given)"`
	Limit           int      `json:"limit,omitempty" jsonschema:"Maximum number of tasks returned (default 50, 0 = no limit); total still counts every hit"`
}

func searchTasksTool() *mcp.Tool {
	schema := mustSchema[searchTasksInput]()
	schema.Properties["query"].MinLength = ptr(1)
	schema.Properties["limit"].Minimum = ptr(0.0)
	schema.Properties["limit"].Default = json.RawMessage("50")
	return &mcp.Tool{
		Name:  "search_tasks",
		Title: "Search Kanboard tasks",
		Description: `Searches tasks in every active project (or only in the given ones) and returns a JSON object: ` +
			`query, projects_searched, total, truncated, tasks [{id, url, title, status, project, column, swimlane, category, ` +
			`assignee, priority, reference, dates, time_tracking, counts, description_excerpt}] newest modification first, and warnings ` +
			`(projects whose search failed). Use get_task for the full ticket. ` +
			`Query syntax: plain words match title and description; filters such as status:open, status:closed, assignee:<username>, ` +
			`creator:<username>, title:"...", description:..., comment:..., tag:..., column:..., category:..., created:>=2026-01-01 ` +
			`can be combined with spaces. Without a status: filter both open and closed tasks match. ` +
			`"me" (assignee:me) is the Kanboard user of this server and works only if it logs in with a personal token.`,
		InputSchema: schema,
		Annotations: readOnly,
	}
}

func (t *tools) searchTasks(_ context.Context, _ *mcp.CallToolRequest, in searchTasksInput) (*mcp.CallToolResult, any, error) {
	res, err := taskdump.Search(t.cfg, taskdump.SearchOptions{
		Query:           in.Query,
		Projects:        in.Projects,
		IncludeInactive: in.IncludeInactive,
		Limit:           in.Limit,
		Workers:         8,
	})
	if err != nil {
		return nil, nil, err
	}
	return textResult(res)
}

// --- get_attachment ---

type getAttachmentInput struct {
	FileId int `json:"file_id" jsonschema:"Attachment id, from a task's attachments or a comment's attachment_ids"`
}

func getAttachmentTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_attachment",
		Title: "Show a Kanboard attachment",
		Description: `Returns the content of a task attachment: images (png, jpeg, gif, webp) as an image, text files as text ` +
			`(cut at 100 KiB). Other file types are refused.`,
		Annotations: readOnly,
	}
}

func (t *tools) getAttachment(_ context.Context, _ *mcp.CallToolRequest, in getAttachmentInput) (*mcp.CallToolResult, any, error) {
	f, err := kanboard.GetTaskFile(t.cfg, in.FileId)
	if err != nil {
		return nil, nil, fmt.Errorf("getTaskFile %v: %w", in.FileId, err)
	}
	if f == nil {
		return nil, nil, fmt.Errorf("attachment %v not found", in.FileId)
	}
	if limit := t.opts.MaxAttachmentBytes; limit > 0 && f.Size > limit {
		return nil, nil, fmt.Errorf("attachment %v %q is %v bytes, more than the %v bytes this server returns; "+
			"download_attachments can save it as a file", f.Id, f.Name, f.Size, limit)
	}
	data, err := kanboard.DownloadTaskFile(t.cfg, f.Id)
	if err != nil {
		return nil, nil, fmt.Errorf("downloadTaskFile %v: %w", f.Id, err)
	}
	about := fmt.Sprintf("Attachment %v %q of task %v, %v bytes", f.Id, f.Name, f.TaskId, len(data))
	mimeType := http.DetectContentType(data)
	switch {
	case slices.Contains(imageTypes, mimeType):
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: about},
			&mcp.ImageContent{Data: data, MIMEType: mimeType},
		}}, nil, nil
	case strings.HasPrefix(mimeType, "text/") && utf8.Valid(data):
		text := data
		if len(text) > MaxTextBytes {
			cut := MaxTextBytes
			for cut > 0 && !utf8.RuneStart(text[cut]) {
				cut--
			}
			text = text[:cut]
			about += fmt.Sprintf(", only the first %v bytes are shown", cut)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: about + ":"},
			&mcp.TextContent{Text: string(text)},
		}}, nil, nil
	}
	return nil, nil, fmt.Errorf("attachment %v %q is %v, which cannot be shown; only images and text files are supported. "+
		"download_attachments can save it as a file", f.Id, f.Name, mimeType)
}

// --- download_attachments ---

type downloadAttachmentsInput struct {
	Tasks   []string `json:"tasks,omitempty" jsonschema:"Save every attachment of these tasks: ids (123), #KB123 references or task URLs"`
	FileIds []int    `json:"file_ids,omitempty" jsonschema:"Save these attachments: ids from a task's attachments or a comment's attachment_ids"`
}

type downloadResult struct {
	Directory string              `json:"directory"`
	Files     []savedAttachment   `json:"files"`
	Skipped   []skippedAttachment `json:"skipped"`
}

type savedAttachment struct {
	FileId    int    `json:"file_id"`
	TaskId    int    `json:"task_id"`
	Name      string `json:"name"`
	SizeBytes int    `json:"size_bytes"`
	MimeType  string `json:"mime_type"`
	LocalPath string `json:"local_path"`
}

// skippedAttachment is a file that was not saved, or a task whose attachments could
// not be listed (then only task_id and reason are set).
type skippedAttachment struct {
	FileId    int    `json:"file_id,omitempty"`
	TaskId    int    `json:"task_id,omitempty"`
	Name      string `json:"name,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Reason    string `json:"reason"`
}

func downloadAttachmentsTool() *mcp.Tool {
	schema := mustSchema[downloadAttachmentsInput]()
	for _, name := range []string{"tasks", "file_ids"} {
		p := schema.Properties[name]
		p.Types, p.Type = nil, "array" // the inferred schema also allows null
	}
	return &mcp.Tool{
		Name:  "download_attachments",
		Title: "Save Kanboard attachments as files",
		Description: `Saves task attachments of any type (docx, pdf, xlsx, zip, images, ...) into local files, ` +
			`as <directory>/<task id>/<file id>-<name>, and returns a JSON object: directory, ` +
			`files [{file_id, task_id, name, size_bytes, mime_type, local_path}] and ` +
			`skipped [{file_id, task_id, name, size_bytes, reason}] for files that were not saved (too large, not found, no access). ` +
			`Give tasks (every attachment of each task), file_ids, or both. The directory is set when the server is started; ` +
			`read or copy the files from local_path with your own tools. An earlier copy of the same file is replaced. ` +
			`To look at an image or a text file directly, get_attachment is simpler.`,
		InputSchema: schema,
		// Writes local files (only below the configured directory), but nothing on Kanboard.
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), IdempotentHint: true},
	}
}

func (t *tools) downloadAttachments(_ context.Context, _ *mcp.CallToolRequest, in downloadAttachmentsInput) (*mcp.CallToolResult, any, error) {
	if t.opts.DownloadDir == "" {
		return nil, nil, errors.New("this server has no download directory; start it with --download-dir")
	}
	if len(in.Tasks) == 0 && len(in.FileIds) == 0 {
		return nil, nil, errors.New("give tasks, file_ids or both")
	}
	taskIds := make([]int, 0, len(in.Tasks))
	for _, ref := range in.Tasks {
		id, err := taskdump.ParseTaskId(ref)
		if err != nil {
			return nil, nil, err
		}
		taskIds = append(taskIds, id)
	}

	res := downloadResult{Directory: t.opts.DownloadDir, Files: []savedAttachment{}, Skipped: []skippedAttachment{}}
	seen := make(map[int]bool)
	save := func(taskId int, f kanboard.KbResponseTaskFile) {
		if seen[f.Id] {
			return
		}
		seen[f.Id] = true
		saved, err := taskdump.SaveAttachment(t.cfg, t.opts.DownloadDir, t.opts.MaxDownloadBytes, taskId, f)
		if err != nil {
			res.Skipped = append(res.Skipped, skippedAttachment{
				FileId: f.Id, TaskId: taskId, Name: f.Name, SizeBytes: f.Size, Reason: accessHint(err).Error()})
			return
		}
		slog.Info("downloaded attachment", "task_id", taskId, "file_id", f.Id, "path", saved.Path, "bytes", saved.Size)
		res.Files = append(res.Files, savedAttachment{
			FileId: f.Id, TaskId: taskId, Name: f.Name, SizeBytes: saved.Size, MimeType: saved.MimeType, LocalPath: saved.Path})
	}

	for _, taskId := range taskIds {
		files, err := t.taskFiles(taskId)
		if err != nil {
			res.Skipped = append(res.Skipped, skippedAttachment{TaskId: taskId, Reason: err.Error()})
			continue
		}
		for _, f := range files {
			save(taskId, f)
		}
	}
	for _, fileId := range in.FileIds {
		f, err := kanboard.GetTaskFile(t.cfg, fileId)
		switch {
		case err != nil:
			res.Skipped = append(res.Skipped, skippedAttachment{FileId: fileId, Reason: accessHint(fmt.Errorf("getTaskFile: %w", err)).Error()})
		case f == nil:
			res.Skipped = append(res.Skipped, skippedAttachment{FileId: fileId, Reason: fmt.Sprintf("attachment %v not found", fileId)})
		default:
			save(f.TaskId, *f)
		}
	}
	return textResult(res)
}

// taskFiles lists the attachments of a task. getTask comes first, so that a missing
// task is reported as such instead of as a task without attachments.
func (t *tools) taskFiles(taskId int) ([]kanboard.KbResponseTaskFile, error) {
	task, err := kanboard.GetTask(t.cfg, taskId)
	if err != nil {
		return nil, accessHint(fmt.Errorf("getTask: %w", err))
	}
	if task == nil {
		return nil, taskdump.ErrNotFound{TaskId: taskId}
	}
	files, err := kanboard.GetAllTaskFiles(t.cfg, taskId)
	if err != nil {
		return nil, accessHint(fmt.Errorf("getAllTaskFiles: %w", err))
	}
	return files, nil
}

// --- helpers ---

// accessHint explains the 403 that a personal token gets for projects its user is not
// a member of.
func accessHint(err error) error {
	if kanboard.IsForbidden(err) {
		return fmt.Errorf("%w (the Kanboard user of this server has no access to the task)", err)
	}
	return err
}

func textResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := taskdump.MarshalJSON(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil, nil
}

func mustSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	return schema
}

func ptr[T any](v T) *T { return &v }
