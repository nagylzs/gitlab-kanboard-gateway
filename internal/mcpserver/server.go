// Package mcpserver exposes the read-only Kanboard lookups of taskdump as MCP tools.
// No tool ever calls a Kanboard procedure that modifies data.
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
}

// MaxTextBytes is where get_attachment cuts text files.
const MaxTextBytes = 100 * 1024

// Image types that MCP clients can be expected to show.
var imageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

const instructions = `Read-only access to a Kanboard server; nothing is ever modified.
- get_task: everything about one or more tasks as JSON. Use fields and max_comments to keep big tickets small.
- search_tasks: find tasks with the Kanboard search syntax; returns short summaries, use get_task for the full ticket.
- get_attachment: shows an attachment, e.g. a screenshot embedded in a comment (see the comment's attachment_ids).`

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
		if kanboard.IsForbidden(err) {
			err = fmt.Errorf("%w (the Kanboard user of this server has no access to the task)", err)
		}
		return nil, err
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
		return nil, nil, fmt.Errorf("attachment %v %q is %v bytes, more than the %v bytes this server returns",
			f.Id, f.Name, f.Size, limit)
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
	return nil, nil, fmt.Errorf("attachment %v %q is %v, which cannot be shown; only images and text files are supported",
		f.Id, f.Name, mimeType)
}

// --- helpers ---

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
