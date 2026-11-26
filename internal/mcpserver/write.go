package mcpserver

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskedit"
)

// The tools in this file change Kanboard. They are added only with Options.Writable.
// None of them deletes a task, a comment or an attachment.

const writableInstructions = `Access to a Kanboard server, reading and changing tasks.
Reading:
` + readTools + `
Changing (in the name of the Kanboard user of this server; descriptions and comments you write get a signature footer, ` +
	`do not write one yourself):
- create_task: new task in a project.
- assign_task: change the assignee; with the Group_assign plugin also the assigned group and the other assignees
  (several users on one task).
- set_task_color: change the color of a task (list_task_colors lists the colors).
- add_comment, update_comment: comment on a task; edit a comment written through this server (not a person's).
- upload_files: attach local files from the upload directory to a task, with a comment that shows them.
- add_task_link, remove_task_link: links between tasks ("blocks", "relates to", ...).
- add_external_link, remove_external_link: URLs attached to a task.
- move_task, close_task, open_task: column, swimlane, position, status.
Comment and link ids are in get_task's output. Change only what you were asked to change.`

var (
	// Adds something new; never changes or removes what is there.
	additive = &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}
	// Changes or removes something that is there.
	changing = &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(false)}
	// Changes something; repeating the call changes nothing more.
	changingIdempotent = &mcp.ToolAnnotations{DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: ptr(false)}
)

func addWriteTools(s *mcp.Server, cfg config.KanboardConfig, opts Options) {
	w := &writeTools{ed: taskedit.NewEditor(cfg, taskedit.Options{
		Signature:      opts.Signature,
		UploadDir:      opts.UploadDir,
		MaxUploadBytes: opts.MaxUploadBytes,
	})}
	mcp.AddTool(s, createTaskTool(), w.createTask)
	mcp.AddTool(s, addCommentTool(), w.addComment)
	mcp.AddTool(s, updateCommentTool(), w.updateComment)
	mcp.AddTool(s, uploadFilesTool(opts.UploadDir), w.uploadFiles)
	mcp.AddTool(s, addTaskLinkTool(), w.addTaskLink)
	mcp.AddTool(s, removeTaskLinkTool(), w.removeTaskLink)
	mcp.AddTool(s, addExternalLinkTool(), w.addExternalLink)
	mcp.AddTool(s, removeExternalLinkTool(), w.removeExternalLink)
	mcp.AddTool(s, assignTaskTool(), w.assignTask)
	mcp.AddTool(s, setTaskColorTool(), w.setTaskColor)
	mcp.AddTool(s, moveTaskTool(), w.moveTask)
	mcp.AddTool(s, setStatusTool("close_task", "Close a Kanboard task", "Closes a task (it stays where it is on the board). "+
		"Closing a closed task changes nothing."), w.closeTask)
	mcp.AddTool(s, setStatusTool("open_task", "Reopen a Kanboard task", "Reopens a closed task. "+
		"Opening an open task changes nothing."), w.openTask)
}

type writeTools struct {
	ed *taskedit.Editor
}

// done logs a change and returns its result.
func done(tool string, res any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		slog.Info("change failed", "tool", tool, "error", err)
		return nil, nil, err
	}
	slog.Info("changed Kanboard", "tool", tool, "result", res)
	return textResult(res)
}

// --- create_task ---

type createTaskInput struct {
	Project     string   `json:"project" jsonschema:"Project id, or its name (or a unique part of it)"`
	Title       string   `json:"title" jsonschema:"Task title"`
	Description string   `json:"description,omitempty" jsonschema:"Task description, markdown"`
	Column      string   `json:"column,omitempty" jsonschema:"Column id or name (default: the first column)"`
	Swimlane    string   `json:"swimlane,omitempty" jsonschema:"Swimlane id or name (default: the first one)"`
	Category    string   `json:"category,omitempty" jsonschema:"Category id or name (default: none)"`
	Owner       string   `json:"owner,omitempty" jsonschema:"Assignee: me, a user id, name or username; must be a project member (default: nobody)"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Tags; new ones are created in the project"`
	Color       string   `json:"color,omitempty" jsonschema:"Color id or name, see list_task_colors (default: the server's default color)"`
	Group       string   `json:"group,omitempty" jsonschema:"Assigned group (needs the Group_assign plugin): group id or name; the group must be added to the project"`
	Others      []string `json:"other_assignees,omitempty" jsonschema:"Other assignees besides the owner (needs the Group_assign plugin): me, user ids, names or usernames of project members"`
}

func createTaskTool() *mcp.Tool {
	schema := mustSchema[createTaskInput]()
	schema.Properties["project"].MinLength = ptr(1)
	schema.Properties["title"].MinLength = ptr(1)
	for _, name := range []string{"tags", "other_assignees"} {
		p := schema.Properties[name]
		p.Types, p.Type = nil, "array"
	}
	return &mcp.Tool{
		Name:  "create_task",
		Title: "Create a Kanboard task",
		Description: `Creates a task and returns a JSON object: id, url, title, status, color, project, column, swimlane, position. ` +
			`With a group or other assignees, also assigned_group and other_assignees. ` +
			`A signature footer is added below the description. Names are matched case-insensitively; ` +
			`a wrong name is refused with the list of valid ones.`,
		InputSchema: schema,
		Annotations: additive,
	}
}

func (w *writeTools) createTask(_ context.Context, _ *mcp.CallToolRequest, in createTaskInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.CreateTask(taskedit.CreateTaskInput{
		Project: in.Project, Title: in.Title, Description: in.Description,
		Column: in.Column, Swimlane: in.Swimlane, Category: in.Category, Owner: in.Owner, Tags: in.Tags,
		Color: in.Color, Group: in.Group, OtherAssignees: in.Others,
	})
	return done("create_task", res, err)
}

// --- assign_task ---

type assignTaskInput struct {
	Task   string   `json:"task" jsonschema:"Task id (123), #KB123 reference or task URL"`
	Owner  string   `json:"owner,omitempty" jsonschema:"New assignee: me, a user id, name or username of a project member; nobody removes the assignee (default: unchanged)"`
	Group  string   `json:"group,omitempty" jsonschema:"Assigned group (Group_assign plugin): group id or name, the group must be added to the project; none removes the group (default: unchanged)"`
	Others []string `json:"other_assignees,omitempty" jsonschema:"Replaces all other assignees (Group_assign plugin): me, user ids, names or usernames of project members; an empty list removes them all (default: unchanged)"`
	Add    []string `json:"add_other_assignees,omitempty" jsonschema:"Users to add to the other assignees; the others stay"`
	Remove []string `json:"remove_other_assignees,omitempty" jsonschema:"Users to remove from the other assignees; the others stay"`
}

func assignTaskTool() *mcp.Tool {
	schema := mustSchema[assignTaskInput]()
	schema.Properties["task"].MinLength = ptr(1)
	for _, name := range []string{"other_assignees", "add_other_assignees", "remove_other_assignees"} {
		p := schema.Properties[name]
		p.Types, p.Type = nil, "array"
	}
	return &mcp.Tool{
		Name:  "assign_task",
		Title: "Assign a Kanboard task",
		Description: `Changes who a task is assigned to: the assignee (owner) and, with Kanboard's Group_assign plugin ` +
			`(1.8.4 or newer), the assigned group and the other assignees, so that several users share the task. ` +
			`Only the given fields change. To add or remove a few other assignees use add_other_assignees / ` +
			`remove_other_assignees; other_assignees replaces the whole list. Nothing is changed when a name is wrong ` +
			`(the error lists the valid ones) or the plugin is missing. Returns a JSON object: id, url, title, owner, ` +
			`assigned_group, other_assignees.`,
		InputSchema: schema,
		Annotations: changingIdempotent,
	}
}

func (w *writeTools) assignTask(_ context.Context, _ *mcp.CallToolRequest, in assignTaskInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.AssignTask(in.Task, taskedit.AssignInput{
		Owner: in.Owner, Group: in.Group, OtherAssignees: in.Others,
		AddOtherAssignees: in.Add, RemoveOtherAssignees: in.Remove,
	})
	return done("assign_task", res, err)
}

// --- set_task_color ---

type setTaskColorInput struct {
	Task  string `json:"task" jsonschema:"Task id (123), #KB123 reference or task URL"`
	Color string `json:"color" jsonschema:"Color id (e.g. red, deep_orange) or name, as list_task_colors shows them"`
}

func setTaskColorTool() *mcp.Tool {
	schema := mustSchema[setTaskColorInput]()
	schema.Properties["task"].MinLength = ptr(1)
	schema.Properties["color"].MinLength = ptr(1)
	return &mcp.Tool{
		Name:  "set_task_color",
		Title: "Set the color of a Kanboard task",
		Description: `Changes the color of a task. color is an id or a name from list_task_colors, case-insensitive ` +
			`(a part of exactly one name also works); an unknown color is refused with the list of ids. ` +
			`Returns a JSON object: id, url, title, color {id, name}, previous_color {id, name}, and warnings, e.g. when ` +
			`an automatic action of the project changed the color again.`,
		InputSchema: schema,
		Annotations: changingIdempotent,
	}
}

func (w *writeTools) setTaskColor(_ context.Context, _ *mcp.CallToolRequest, in setTaskColorInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.SetTaskColor(in.Task, in.Color)
	return done("set_task_color", res, err)
}

// --- comments ---

type addCommentInput struct {
	Task    string `json:"task" jsonschema:"Task id (123), #KB123 reference or task URL"`
	Content string `json:"content" jsonschema:"Comment text, markdown, without a signature"`
}

func addCommentTool() *mcp.Tool {
	schema := mustSchema[addCommentInput]()
	schema.Properties["content"].MinLength = ptr(1)
	return &mcp.Tool{
		Name:  "add_comment",
		Title: "Comment on a Kanboard task",
		Description: `Adds a comment to a task, with a signature footer below it. ` +
			`Returns a JSON object: comment_id, task_id, content (as stored).`,
		InputSchema: schema,
		Annotations: additive,
	}
}

func (w *writeTools) addComment(_ context.Context, _ *mcp.CallToolRequest, in addCommentInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.AddComment(in.Task, in.Content)
	return done("add_comment", res, err)
}

type updateCommentInput struct {
	CommentId int    `json:"comment_id" jsonschema:"Comment id, from get_task's comments"`
	Content   string `json:"content" jsonschema:"The complete new text, markdown, without the signature footer"`
}

func updateCommentTool() *mcp.Tool {
	schema := mustSchema[updateCommentInput]()
	schema.Properties["content"].MinLength = ptr(1)
	schema.Properties["comment_id"].Minimum = ptr(1.0)
	return &mcp.Tool{
		Name:  "update_comment",
		Title: "Edit a Kanboard comment",
		Description: `Replaces the text of a comment. Only signed comments written through this server can be edited: ` +
			`comments of other users, and comments without a signature footer (written by the person who owns the account) are refused, ` +
			`and so is every edit when the server runs without a signature. ` +
			`The footer is kept and extended with "modified by". Returns a JSON object: comment_id, task_id, content (as stored).`,
		InputSchema: schema,
		Annotations: changing,
	}
}

func (w *writeTools) updateComment(_ context.Context, _ *mcp.CallToolRequest, in updateCommentInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.UpdateComment(in.CommentId, in.Content)
	return done("update_comment", res, err)
}

// --- upload_files ---

type uploadFilesInput struct {
	Task    string   `json:"task" jsonschema:"Task id (123), #KB123 reference or task URL"`
	Files   []string `json:"files" jsonschema:"Local files in the upload directory: absolute paths, or paths relative to the upload directory"`
	Comment string   `json:"comment,omitempty" jsonschema:"Comment text above the list of files, markdown (default: New file(s):)"`
}

func uploadFilesTool(dir string) *mcp.Tool {
	schema := mustSchema[uploadFilesInput]()
	files := schema.Properties["files"]
	files.Types, files.Type = nil, "array"
	files.MinItems = ptr(1)
	where := "This server has no upload directory, so this tool fails."
	if dir != "" {
		where = fmt.Sprintf("The upload directory is %v; copy files there first, files elsewhere are refused.", dir)
	}
	return &mcp.Tool{
		Name:  "upload_files",
		Title: "Attach files to a Kanboard task",
		Description: `Uploads local files as attachments of a task and adds one comment about them: the given comment text, ` +
			`then the files (images shown inline, other files as download links), then the signature footer. ` + where + ` ` +
			`Returns a JSON object: task_id, files [{file_id, name, size_bytes, path}], comment_id, comment.`,
		InputSchema: schema,
		Annotations: additive,
	}
}

func (w *writeTools) uploadFiles(_ context.Context, _ *mcp.CallToolRequest, in uploadFilesInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.UploadFiles(in.Task, in.Files, in.Comment)
	return done("upload_files", res, err)
}

// --- internal links ---

type addTaskLinkInput struct {
	Task      string `json:"task" jsonschema:"Task id (123), #KB123 reference or task URL"`
	Relation  string `json:"relation" jsonschema:"How task relates to other_task, e.g. relates to, blocks, is blocked by, duplicates, is a child of, is a parent of, fixes, is fixed by"`
	OtherTask string `json:"other_task" jsonschema:"The other task: id, #KB123 reference or task URL"`
}

func addTaskLinkTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "add_task_link",
		Title: "Link two Kanboard tasks",
		Description: `Links two tasks as "<task> <relation> <other_task>", e.g. 12 blocks 34. Kanboard adds the opposite link ` +
			`(34 is blocked by 12) to the other task. An unknown relation is refused with the list of valid ones. ` +
			`Returns a JSON object: link_id, task_id, relation, other_task_id.`,
		Annotations: additive,
	}
}

func (w *writeTools) addTaskLink(_ context.Context, _ *mcp.CallToolRequest, in addTaskLinkInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.AddTaskLink(in.Task, in.OtherTask, in.Relation)
	return done("add_task_link", res, err)
}

type removeLinkInput struct {
	Task   string `json:"task" jsonschema:"Task id (123), #KB123 reference or task URL"`
	LinkId int    `json:"link_id" jsonschema:"Link id, from get_task's links (or external_links) of this task"`
}

func removeTaskLinkTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "remove_task_link",
		Title: "Remove a link between Kanboard tasks",
		Description: `Removes an internal link of the task (an entry of get_task's links), and its opposite on the other task. ` +
			`Returns a JSON object describing the removed link: link_id, task_id, relation, other_task_id.`,
		Annotations: changing,
	}
}

func (w *writeTools) removeTaskLink(_ context.Context, _ *mcp.CallToolRequest, in removeLinkInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.RemoveTaskLink(in.Task, in.LinkId)
	return done("remove_task_link", res, err)
}

// --- external links ---

type addExternalLinkInput struct {
	Task  string `json:"task" jsonschema:"Task id (123), #KB123 reference or task URL"`
	Url   string `json:"url" jsonschema:"Absolute URL"`
	Title string `json:"title,omitempty" jsonschema:"Link title (default: Kanboard fetches the page and uses its title)"`
}

func addExternalLinkTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "add_external_link",
		Title: "Attach a URL to a Kanboard task",
		Description: `Adds an external link (a URL, e.g. a merge request or a document) to a task. ` +
			`Returns a JSON object: link_id, task_id, url, title.`,
		Annotations: additive,
	}
}

func (w *writeTools) addExternalLink(_ context.Context, _ *mcp.CallToolRequest, in addExternalLinkInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.AddExternalLink(in.Task, in.Url, in.Title)
	return done("add_external_link", res, err)
}

func removeExternalLinkTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "remove_external_link",
		Title: "Remove a URL from a Kanboard task",
		Description: `Removes an external link of the task (an entry of get_task's external_links). ` +
			`Returns a JSON object describing the removed link: link_id, task_id, url, title.`,
		Annotations: changing,
	}
}

func (w *writeTools) removeExternalLink(_ context.Context, _ *mcp.CallToolRequest, in removeLinkInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.RemoveExternalLink(in.Task, in.LinkId)
	return done("remove_external_link", res, err)
}

// --- move, close, open ---

type moveTaskInput struct {
	Task     string `json:"task" jsonschema:"Task id (123), #KB123 reference or task URL"`
	Column   string `json:"column,omitempty" jsonschema:"Column id or name (default: the current column)"`
	Swimlane string `json:"swimlane,omitempty" jsonschema:"Swimlane id or name (default: the current swimlane)"`
	Position int    `json:"position,omitempty" jsonschema:"Position in the column, 1 = top (default: the bottom)"`
}

func moveTaskTool() *mcp.Tool {
	schema := mustSchema[moveTaskInput]()
	schema.Properties["position"].Minimum = ptr(0.0)
	return &mcp.Tool{
		Name:  "move_task",
		Title: "Move a Kanboard task",
		Description: `Moves a task to another column and/or swimlane of its project, or to another position in its column; ` +
			`give at least one of them. A wrong name is refused with the list of valid ones. ` +
			`Returns a JSON object: id, url, title, status, color, project, column, swimlane, position.`,
		InputSchema: schema,
		Annotations: changing,
	}
}

func (w *writeTools) moveTask(_ context.Context, _ *mcp.CallToolRequest, in moveTaskInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.MoveTask(in.Task, in.Column, in.Swimlane, in.Position)
	return done("move_task", res, err)
}

type taskInput struct {
	Task string `json:"task" jsonschema:"Task id (123), #KB123 reference or task URL"`
}

func setStatusTool(name, title, description string) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description + ` Returns a JSON object: id, url, title, status, color, project, column, swimlane, position.`,
		Annotations: changingIdempotent,
	}
}

func (w *writeTools) closeTask(_ context.Context, _ *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.CloseTask(in.Task)
	return done("close_task", res, err)
}

func (w *writeTools) openTask(_ context.Context, _ *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, any, error) {
	res, err := w.ed.OpenTask(in.Task)
	return done("open_task", res, err)
}
