package taskedit

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskdump"
)

// --- create a task ---

type CreateTaskInput struct {
	Project     string   // id or name
	Title       string   // required
	Description string   // markdown
	Column      string   // id or name; default: the first column
	Swimlane    string   // id or name; default: the default swimlane
	Category    string   // id or name; default: none
	Owner       string   // assignee: "me", id, name or username; default: nobody
	Tags        []string // created in the project when they do not exist yet
	Color       string   // color id or name (see findColor); default: the server's default color
	// Group_assign plugin: the assigned group (id or name) and other assignees (like Owner).
	Group          string
	OtherAssignees []string
}

// TaskResult describes the task after a change.
type TaskResult struct {
	Id       int                `json:"id"`
	Url      string             `json:"url"`
	Title    string             `json:"title"`
	Status   string             `json:"status"`
	Color    string             `json:"color,omitempty"` // color id
	Project  taskdump.NamedRef  `json:"project"`
	Column   *taskdump.NamedRef `json:"column"`
	Swimlane *taskdump.NamedRef `json:"swimlane"`
	Position int                `json:"position"`
	// Only when create_task was given a group or other assignees (read back).
	AssignedGroup  *taskdump.NamedRef `json:"assigned_group,omitempty"`
	OtherAssignees []taskdump.UserRef `json:"other_assignees,omitempty"`
	Warnings       []string           `json:"warnings,omitempty"`
}

func (e *Editor) CreateTask(in CreateTaskInput) (*TaskResult, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, errors.New("the title must not be empty")
	}
	p, err := e.project(in.Project)
	if err != nil {
		return nil, err
	}
	projectId := int(p.Id)
	params := kanboard.KbCreateTaskParams{
		Title:       title,
		ProjectId:   projectId,
		Description: e.signCreated(in.Description),
	}
	for _, t := range in.Tags {
		if t = strings.TrimSpace(t); t != "" {
			params.Tags = append(params.Tags, t)
		}
	}
	var color *taskdump.Color
	if in.Color != "" {
		colors, err := taskdump.ListColors(e.cfg)
		if err != nil {
			return nil, err
		}
		c, err := findColor(in.Color, colors.Colors)
		if err != nil {
			return nil, err
		}
		color, params.ColorId = &c, c.Id
	}
	cols, err := e.columns(projectId)
	if err != nil {
		return nil, err
	}
	lanes, err := e.swimlanes(projectId)
	if err != nil {
		return nil, err
	}
	if in.Column != "" {
		c, err := pick("column", in.Column, cols)
		if err != nil {
			return nil, err
		}
		params.ColumnId = c.id
	}
	if in.Swimlane != "" {
		s, err := pick("swimlane", in.Swimlane, lanes)
		if err != nil {
			return nil, err
		}
		params.SwimlaneId = s.id
	}
	if in.Category != "" {
		cats, err := e.categories(projectId)
		if err != nil {
			return nil, err
		}
		c, err := pick("category", in.Category, cats)
		if err != nil {
			return nil, err
		}
		params.CategoryId = c.id
	}
	if in.Owner != "" {
		o, err := e.member(projectId, in.Owner)
		if err != nil {
			return nil, err
		}
		params.OwnerId = o.id
	}
	if in.Group != "" {
		if params.GroupId, err = e.groupId(in.Group); err != nil {
			return nil, err
		}
	}
	if refs := nonEmpty(in.OtherAssignees); len(refs) > 0 {
		ms, err := e.members(projectId, refs)
		if err != nil {
			return nil, err
		}
		params.OtherAssignees = ids(ms)
	}
	if !e.cfg.IsUserAuth() {
		// A personal token makes its user the creator. With the application token the
		// creator is Kanboard.UserId, but Kanboard refuses a creator who is not a
		// project member, so then the task has no creator.
		if uid, err := e.UserId(); err == nil {
			if ok, err := e.isMember(projectId, uid); err == nil && ok {
				params.CreatorId = uid
			}
		}
	}
	grouped := params.GroupId != 0 || len(params.OtherAssignees) > 0
	create, hint := kanboard.CreateTask, "check that the owner is a member of the project"
	if grouped {
		create, hint = kanboard.CreateTaskGroupAssign, hintGroup+"; the same for the owner"
	}
	id, err := create(e.cfg, params)
	if err != nil {
		if kanboard.IsMethodNotFound(err) {
			return nil, fmt.Errorf("no task was created: a group or other assignees need the Group_assign plugin "+
				"on the Kanboard server: %w", err)
		}
		return nil, refused(err, hint)
	}
	res, err := e.taskResult(id, cols, lanes)
	if err == nil && grouped {
		a := e.assignResult(id)
		res.AssignedGroup, res.OtherAssignees, res.Warnings = a.AssignedGroup, a.OtherAssignees, a.Warnings
	}
	if err == nil && color != nil {
		if w := colorChanged(color.Id, res.Color); w != "" {
			res.Warnings = append(res.Warnings, w)
		}
	}
	return res, err
}

// taskResult reloads a task after a change; cols and lanes may be nil.
func (e *Editor) taskResult(taskId int, cols, lanes []named) (*TaskResult, error) {
	t, err := kanboard.GetTask(e.cfg, taskId)
	if err != nil || t == nil {
		// The change was made; only the summary is missing.
		return &TaskResult{Id: taskId}, nil
	}
	// Names that cannot be loaded are left out of the summary.
	if cols == nil {
		cols, _ = e.columns(t.ProjectId)
	}
	if lanes == nil {
		lanes, _ = e.swimlanes(t.ProjectId)
	}
	res := &TaskResult{
		Id:       t.Id,
		Url:      t.Url,
		Title:    t.Title,
		Status:   status(t.IsActive),
		Color:    t.ColorId,
		Project:  taskdump.NamedRef{Id: t.ProjectId},
		Position: t.Position,
	}
	if p, err := kanboard.GetProjectById(e.cfg, t.ProjectId); err == nil && p != nil {
		res.Project.Name = p.Name
	}
	if name := nameOf(cols, t.ColumnId); name != "" {
		res.Column = &taskdump.NamedRef{Id: t.ColumnId, Name: name}
	}
	if name := nameOf(lanes, t.SwimlaneId); name != "" {
		res.Swimlane = &taskdump.NamedRef{Id: t.SwimlaneId, Name: name}
	}
	return res, nil
}

func status(isActive bool) string {
	if isActive {
		return "open"
	}
	return "closed"
}

// --- comments ---

type CommentResult struct {
	CommentId int    `json:"comment_id"`
	TaskId    int    `json:"task_id"`
	Content   string `json:"content"` // as stored, with the signature footer
}

func (e *Editor) AddComment(taskRef, content string) (*CommentResult, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("the comment must not be empty")
	}
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}
	return e.addComment(t.Id, e.signCreated(content))
}

func (e *Editor) addComment(taskId int, signed string) (*CommentResult, error) {
	uid, err := e.UserId()
	if err != nil {
		return nil, err
	}
	id, err := kanboard.CreateComment(e.cfg, taskId, uid, signed)
	if err != nil {
		return nil, fmt.Errorf("createComment on task %v: %w", taskId, err)
	}
	return &CommentResult{CommentId: id, TaskId: taskId, Content: signed}, nil
}

// UpdateComment replaces the text of a comment. Only comments of the Kanboard user of
// this server that carry a signature footer can be edited, i.e. those written through
// this tool, not by the human owner of the token. Without a signature the two cannot
// be told apart, so then nothing can be edited.
func (e *Editor) UpdateComment(commentId int, content string) (*CommentResult, error) {
	if e.opts.Signature == "" {
		return nil, errors.New("editing comments needs a signature (--signature): only signed comments can be edited, " +
			"so that a comment written by a person is never changed")
	}
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("the comment must not be empty")
	}
	c, err := kanboard.GetComment(e.cfg, commentId)
	if err != nil {
		if kanboard.IsForbidden(err) {
			return nil, fmt.Errorf("comment %v does not exist, or the Kanboard user of this server has no access to it: %w", commentId, err)
		}
		return nil, fmt.Errorf("getComment %v: %w", commentId, err)
	}
	if c == nil {
		return nil, fmt.Errorf("comment %v not found", commentId)
	}
	uid, err := e.UserId()
	if err != nil {
		return nil, err
	}
	if c.UserId != uid {
		author := c.Name
		if author == "" {
			author = c.Username
		}
		return nil, fmt.Errorf("comment %v was written by %q (user %v), not by the Kanboard user of this server (user %v); "+
			"only its own comments can be edited", commentId, author, c.UserId, uid)
	}
	if _, _, ok := splitFooter(c.Comment); !ok {
		return nil, fmt.Errorf("comment %v has no signature footer (\"created by ...\"), so it was written by a person, "+
			"not through this tool; only signed comments can be edited", commentId)
	}
	signed := e.signModified(c.Comment, content)
	if err := kanboard.UpdateComment(e.cfg, commentId, signed); err != nil {
		return nil, fmt.Errorf("updateComment %v: %w", commentId, err)
	}
	return &CommentResult{CommentId: commentId, TaskId: c.TaskId, Content: signed}, nil
}

// --- internal links ---

type TaskLinkResult struct {
	LinkId      int    `json:"link_id"`
	TaskId      int    `json:"task_id"`
	Relation    string `json:"relation"`
	OtherTaskId int    `json:"other_task_id"`
}

// AddTaskLink makes "task <relation> other", e.g. "12 blocks 34". Kanboard also adds
// the opposite link ("34 is blocked by 12") to the other task.
func (e *Editor) AddTaskLink(taskRef, otherRef, relation string) (*TaskLinkResult, error) {
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}
	other, err := e.task(otherRef)
	if err != nil {
		return nil, err
	}
	if t.Id == other.Id {
		return nil, errors.New("a task cannot be linked to itself")
	}
	rel, verified, err := e.linkType(relation)
	if err != nil {
		return nil, err
	}
	id, err := kanboard.CreateTaskLink(e.cfg, t.Id, other.Id, rel.id)
	if err != nil {
		return nil, refused(err, "the tasks may already be linked with this relation")
	}
	if !verified {
		if err := e.checkLinkLabel(t.Id, id, rel.name); err != nil {
			return nil, err
		}
	}
	return &TaskLinkResult{LinkId: id, TaskId: t.Id, Relation: rel.name, OtherTaskId: other.Id}, nil
}

// checkLinkLabel makes sure that a link created with an id from defaultLinks has the
// intended relation, and removes it if it does not (the server has its own link types).
func (e *Editor) checkLinkLabel(taskId, linkId int, want string) error {
	links, err := kanboard.GetAllTaskLinks(e.cfg, taskId)
	if err != nil {
		return fmt.Errorf("link %v was created, but its relation could not be checked: getAllTaskLinks: %w", linkId, err)
	}
	for _, l := range links {
		if l.Id != linkId {
			continue
		}
		if strings.EqualFold(l.Label, want) {
			return nil
		}
		if err := kanboard.RemoveTaskLink(e.cfg, linkId); err != nil {
			return fmt.Errorf("link %v was created as %q instead of %q, and removing it failed: %w", linkId, l.Label, want, err)
		}
		return fmt.Errorf("this Kanboard has its own link types, which only an admin can list; %q could not be created "+
			"(the relations in get_task's links show the labels in use)", want)
	}
	return fmt.Errorf("link %v was created, but its relation could not be checked: it is not among the links of task %v", linkId, taskId)
}

// RemoveTaskLink removes an internal link of the task (and its opposite on the other
// task). The link must belong to the task, so a wrong id cannot remove some other link.
func (e *Editor) RemoveTaskLink(taskRef string, linkId int) (*TaskLinkResult, error) {
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}
	l, err := kanboard.GetTaskLinkById(e.cfg, linkId)
	if err != nil && !kanboard.IsForbidden(err) {
		return nil, fmt.Errorf("getTaskLinkById %v: %w", linkId, err)
	}
	if l == nil || l.TaskId != t.Id {
		return nil, fmt.Errorf("task %v has no internal link %v; the link ids are in get_task's links", t.Id, linkId)
	}
	if err := kanboard.RemoveTaskLink(e.cfg, linkId); err != nil {
		return nil, fmt.Errorf("removeTaskLink %v: %w", linkId, err)
	}
	return &TaskLinkResult{LinkId: linkId, TaskId: t.Id, Relation: l.Label, OtherTaskId: l.OppositeTaskId}, nil
}

// --- external links ---

type ExternalLinkResult struct {
	LinkId int    `json:"link_id"`
	TaskId int    `json:"task_id"`
	Url    string `json:"url"`
	Title  string `json:"title"`
}

func (e *Editor) AddExternalLink(taskRef, rawUrl, title string) (*ExternalLinkResult, error) {
	rawUrl = strings.TrimSpace(rawUrl)
	u, err := url.Parse(rawUrl)
	if err != nil || u.Scheme == "" || (u.Host == "" && u.Opaque == "") {
		return nil, fmt.Errorf("%q is not an absolute URL", rawUrl)
	}
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}
	id, err := kanboard.CreateExternalTaskLink(e.cfg, t.Id, rawUrl, strings.TrimSpace(title))
	if err != nil {
		return nil, fmt.Errorf("createExternalTaskLink: %w", err)
	}
	res := &ExternalLinkResult{LinkId: id, TaskId: t.Id, Url: rawUrl, Title: title}
	// Without a title Kanboard uses the page title; report what it chose.
	if links, err := kanboard.GetAllExternalTaskLinks(e.cfg, t.Id); err == nil {
		for _, l := range links {
			if l.Id == id {
				res.Title = l.Title
			}
		}
	}
	return res, nil
}

func (e *Editor) RemoveExternalLink(taskRef string, linkId int) (*ExternalLinkResult, error) {
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}
	links, err := kanboard.GetAllExternalTaskLinks(e.cfg, t.Id)
	if err != nil {
		return nil, fmt.Errorf("getAllExternalTaskLinks: %w", err)
	}
	for _, l := range links {
		if l.Id != linkId {
			continue
		}
		if err := kanboard.RemoveExternalTaskLink(e.cfg, t.Id, linkId); err != nil {
			return nil, fmt.Errorf("removeExternalTaskLink %v: %w", linkId, err)
		}
		return &ExternalLinkResult{LinkId: linkId, TaskId: t.Id, Url: l.Url, Title: l.Title}, nil
	}
	return nil, fmt.Errorf("task %v has no external link %v; the link ids are in get_task's external_links", t.Id, linkId)
}

// --- move, close, open ---

// bottom is a position past the end of any column; Kanboard puts the task last.
const bottom = 1_000_000

// MoveTask moves a task to another column and/or swimlane of its project, or to
// another position. Empty column or swimlane keeps the current one; position 0 means
// the bottom of the column.
func (e *Editor) MoveTask(taskRef, column, swimlane string, position int) (*TaskResult, error) {
	if column == "" && swimlane == "" && position == 0 {
		return nil, errors.New("give a column, a swimlane or a position")
	}
	if position < 0 {
		return nil, errors.New("position starts at 1 (the top of the column)")
	}
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}
	cols, err := e.columns(t.ProjectId)
	if err != nil {
		return nil, err
	}
	lanes, err := e.swimlanes(t.ProjectId)
	if err != nil {
		return nil, err
	}
	columnId, swimlaneId := t.ColumnId, t.SwimlaneId
	if column != "" {
		c, err := pick("column", column, cols)
		if err != nil {
			return nil, err
		}
		columnId = c.id
	}
	if swimlane != "" {
		s, err := pick("swimlane", swimlane, lanes)
		if err != nil {
			return nil, err
		}
		swimlaneId = s.id
	}
	if position == 0 {
		position = bottom
	}
	if err := kanboard.MoveTaskPosition(e.cfg, t.ProjectId, t.Id, columnId, position, swimlaneId); err != nil {
		return nil, fmt.Errorf("moveTaskPosition: %w", err)
	}
	return e.taskResult(t.Id, cols, lanes)
}

func (e *Editor) CloseTask(taskRef string) (*TaskResult, error) {
	return e.setStatus(taskRef, false)
}

func (e *Editor) OpenTask(taskRef string) (*TaskResult, error) {
	return e.setStatus(taskRef, true)
}

func (e *Editor) setStatus(taskRef string, open bool) (*TaskResult, error) {
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}
	if t.IsActive != open {
		change, method := kanboard.CloseTask, "closeTask"
		if open {
			change, method = kanboard.OpenTask, "openTask"
		}
		if err := change(e.cfg, t.Id); err != nil {
			return nil, fmt.Errorf("%v %v: %w", method, t.Id, err)
		}
	}
	return e.taskResult(t.Id, nil, nil)
}
