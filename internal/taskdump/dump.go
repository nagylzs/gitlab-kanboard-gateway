package taskdump

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
)

type Options struct {
	// Directory to download attachments into (files land in <dir>/<taskId>/).
	// Empty means: do not download, only list attachment metadata.
	DownloadDir string
	// Attachments larger than this are not downloaded (0 = no limit).
	MaxDownloadBytes int64
}

// Dumper caches lookups (users, columns, ...) across multiple tasks.
type Dumper struct {
	cfg   config.KanboardConfig
	opts  Options
	users map[int]*UserRef
}

func NewDumper(cfg config.KanboardConfig, opts Options) *Dumper {
	return &Dumper{cfg: cfg, opts: opts, users: make(map[int]*UserRef)}
}

// ErrNotFound is returned by Dump when the task id does not exist.
type ErrNotFound struct{ TaskId int }

func (e ErrNotFound) Error() string { return fmt.Sprintf("task %v not found", e.TaskId) }

// Dump collects everything about one task. A failing getTask is fatal; failures of
// the secondary calls are recorded in doc.Warnings so that a partial document is
// still returned.
func (d *Dumper) Dump(taskId int) (*TaskDocument, error) {
	task, err := kanboard.GetTask(d.cfg, taskId)
	if err != nil {
		return nil, fmt.Errorf("getTask %v: %w", taskId, err)
	}
	if task == nil {
		return nil, ErrNotFound{TaskId: taskId}
	}

	doc := &TaskDocument{
		Id:          task.Id,
		Url:         task.Url,
		Title:       task.Title,
		Description: task.Description,
		Status:      openClosed(task.IsActive),
		Project:     ProjectRef{Id: task.ProjectId},
		Color:       task.ColorId,
		Priority:    task.Priority,
		Score:       int(task.Score),
		Reference:   task.Reference,
		Position:    task.Position,
		Dates: Dates{
			Created:   tsPtr(task.DateCreation),
			Modified:  tsPtr(task.DateModification),
			Moved:     tsPtr(task.DateMoved),
			Started:   tsPtr(task.DateStarted),
			Due:       tsPtr(task.DateDue),
			Completed: tsPtr(task.DateCompleted),
		},
		TimeTracking:  TimeTracking{SpentHours: task.TimeSpent, EstimatedHours: task.TimeEstimated},
		Tags:          []string{},
		Metadata:      map[string]string{},
		Subtasks:      []Subtask{},
		Comments:      []Comment{},
		Attachments:   []Attachment{},
		Links:         []Link{},
		ExternalLinks: []ExternalLink{},
		FetchedAt:     time.Now().Format(time.RFC3339),
	}
	warn := func(what string, err error) {
		msg := fmt.Sprintf("%v: %v", what, err)
		slog.Warn(msg, "task_id", taskId)
		doc.Warnings = append(doc.Warnings, msg)
	}

	if task.RecurrenceStatus != 0 {
		doc.Recurrence = &Recurrence{
			Status:    task.RecurrenceStatus,
			Trigger:   task.RecurrenceTrigger,
			Factor:    task.RecurrenceFactor,
			Timeframe: int(task.RecurrenceTimeframe),
			Basedate:  derefInt(task.RecurrenceBasedate),
			ParentId:  anyToIntPtr(task.RecurrenceParent),
			ChildId:   anyToIntPtr(task.RecurrenceChild),
		}
	}
	if s, ok := task.ExternalProvider.(string); ok && s != "" {
		uri, _ := task.ExternalUri.(string)
		doc.External = &ExternalTask{Provider: s, Uri: uri}
	}

	// --- reference lookups ---
	if project, err := kanboard.GetProjectById(d.cfg, task.ProjectId); err != nil {
		warn("getProjectById", err)
	} else if project != nil {
		doc.Project = ProjectRef{
			Id:         int(project.Id),
			Name:       project.Name,
			Identifier: project.Identifier,
			BoardUrl:   project.Url.Board,
		}
	}
	if col, err := kanboard.GetColumn(d.cfg, task.ColumnId); err != nil {
		warn("getColumn", err)
	} else if col != nil {
		doc.Column = &NamedRef{Id: int(col.Id), Name: col.Title}
	}
	if task.SwimlaneId != 0 {
		if sw, err := kanboard.GetSwimlane(d.cfg, task.SwimlaneId); err != nil {
			warn("getSwimlane", err)
		} else if sw != nil {
			doc.Swimlane = &NamedRef{Id: int(sw.Id), Name: sw.Name}
		}
	}
	if task.CategoryId != 0 {
		if cat, err := kanboard.GetCategory(d.cfg, task.CategoryId); err != nil {
			warn("getCategory", err)
		} else if cat != nil {
			doc.Category = &NamedRef{Id: int(cat.Id), Name: cat.Name}
		}
	}
	doc.Owner = d.user(task.OwnerId, warn)
	doc.Creator = d.user(task.CreatorId, warn)

	// --- tags & metadata ---
	if tags, err := kanboard.GetTaskTags(d.cfg, taskId); err != nil {
		warn("getTaskTags", err)
	} else {
		for _, name := range tags {
			doc.Tags = append(doc.Tags, name)
		}
	}
	if meta, err := kanboard.GetTaskMetadata(d.cfg, taskId); err != nil {
		warn("getTaskMetadata", err)
	} else {
		doc.Metadata = meta
	}

	// --- subtasks ---
	if subtasks, err := kanboard.GetAllSubtasks(d.cfg, taskId); err != nil {
		warn("getAllSubtasks", err)
	} else {
		for _, st := range subtasks {
			doc.Subtasks = append(doc.Subtasks, Subtask{
				Id:           st.Id,
				Title:        st.Title,
				Status:       subtaskStatus(st.Status),
				Assignee:     userFromParts(st.UserId, st.Username, st.Name),
				TimeTracking: TimeTracking{SpentHours: st.TimeSpent, EstimatedHours: st.TimeEstimated},
			})
		}
	}

	// --- attachments (before comments: comment text may embed them) ---
	attByID := make(map[int]*Attachment)
	if files, err := kanboard.GetAllTaskFiles(d.cfg, taskId); err != nil {
		warn("getAllTaskFiles", err)
	} else {
		for _, f := range files {
			att := Attachment{
				Id:         f.Id,
				Name:       f.Name,
				SizeBytes:  f.Size,
				IsImage:    f.IsImage,
				Uploaded:   tsPtr(f.Date),
				UploadedBy: userFromParts(f.UserId, f.Username, f.UserName),
			}
			if d.opts.DownloadDir != "" {
				d.download(taskId, f, &att)
			}
			doc.Attachments = append(doc.Attachments, att)
			attByID[att.Id] = &doc.Attachments[len(doc.Attachments)-1]
		}
	}

	// --- comments ---
	if comments, err := kanboard.GetAllComments(d.cfg, taskId); err != nil {
		warn("getAllComments", err)
	} else {
		for _, c := range comments {
			cm := Comment{
				Id:         c.Id,
				Created:    tsPtr(c.DateCreation),
				Modified:   tsPtr(c.DateModification),
				Author:     UserRef{Id: c.UserId, Username: c.Username, Name: c.Name, Email: c.Email},
				Visibility: c.Visibility,
			}
			cm.Content, cm.AttachmentIds = rewriteEmbeddedFiles(c.Comment, appBaseUrl(d.cfg.ApiUrl), attByID)
			doc.Comments = append(doc.Comments, cm)
		}
	}

	// --- links ---
	if links, err := kanboard.GetAllTaskLinks(d.cfg, taskId); err != nil {
		warn("getAllTaskLinks", err)
	} else {
		for _, l := range links {
			doc.Links = append(doc.Links, Link{
				Id:       l.Id,
				Relation: l.Label,
				Task: LinkedTask{
					Id:       l.TaskId,
					Title:    l.Title,
					Status:   openClosed(l.IsActive),
					Project:  NamedRef{Id: l.ProjectId, Name: l.ProjectName},
					Column:   l.ColumnTitle,
					Assignee: userFromParts(l.TaskAssigneeId, l.TaskAssigneeUsername, l.TaskAssigneeName),
					Dates: LinkedDates{
						Started:   tsPtr(l.DateStarted),
						Due:       tsPtr(l.DateDue),
						Completed: tsPtr(l.DateCompleted),
					},
					TimeTracking: TimeTracking{SpentHours: l.TaskTimeSpent, EstimatedHours: l.TaskTimeEstimated},
				},
			})
		}
	}
	if ext, err := kanboard.GetAllExternalTaskLinks(d.cfg, taskId); err != nil {
		warn("getAllExternalTaskLinks", err)
	} else {
		for _, l := range ext {
			doc.ExternalLinks = append(doc.ExternalLinks, ExternalLink{
				Id:       l.Id,
				Type:     l.LinkType,
				Relation: l.Dependency,
				Title:    l.Title,
				Url:      l.Url,
				Created:  tsPtr(l.DateCreation),
				Modified: tsPtr(l.DateModification),
				Creator:  userFromParts(l.CreatorId, l.CreatorUsername, l.CreatorName),
			})
		}
	}

	return doc, nil
}

func (d *Dumper) download(taskId int, f kanboard.KbResponseTaskFile, att *Attachment) {
	if d.opts.MaxDownloadBytes > 0 && f.Size > d.opts.MaxDownloadBytes {
		att.DownloadError = fmt.Sprintf("skipped: size %v exceeds limit %v bytes", f.Size, d.opts.MaxDownloadBytes)
		return
	}
	dir := filepath.Join(d.opts.DownloadDir, fmt.Sprintf("%v", taskId))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		att.DownloadError = err.Error()
		return
	}
	target := filepath.Join(dir, fmt.Sprintf("%v-%v", f.Id, sanitizeFileName(f.Name)))
	data, err := kanboard.DownloadTaskFile(d.cfg, f.Id)
	if err != nil {
		att.DownloadError = err.Error()
		return
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		att.DownloadError = err.Error()
		return
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		abs = target
	}
	att.LocalPath = abs
	slog.Info("downloaded attachment", "task_id", taskId, "file_id", f.Id, "path", abs, "bytes", len(data))
}

// user resolves a user id via the API (cached). Id 0 means "nobody".
func (d *Dumper) user(id int, warn func(string, error)) *UserRef {
	if id == 0 {
		return nil
	}
	if u, ok := d.users[id]; ok {
		return u
	}
	u, err := kanboard.GetUser(d.cfg, id)
	if err != nil {
		warn(fmt.Sprintf("getUser %v", id), err)
		return &UserRef{Id: id}
	}
	var ref *UserRef
	if u != nil {
		ref = &UserRef{Id: u.Id, Username: u.Username, Name: u.Name, Email: u.Email}
	} else {
		ref = &UserRef{Id: id}
	}
	d.users[id] = ref
	return ref
}

// --- helpers ---

func openClosed(isActive bool) string {
	if isActive {
		return "open"
	}
	return "closed"
}

func subtaskStatus(s int) string {
	switch s {
	case 0:
		return "todo"
	case 1:
		return "in_progress"
	case 2:
		return "done"
	}
	return fmt.Sprintf("unknown(%v)", s)
}

func userFromParts(id int, username *string, name *string) *UserRef {
	if id == 0 {
		return nil
	}
	return &UserRef{Id: id, Username: derefStr(username), Name: derefStr(name)}
}

// tsPtr converts a unix timestamp (int or *int) to RFC3339 in local time; 0/nil -> nil.
func tsPtr[T int | *int](v T) *string {
	var n int
	switch x := any(v).(type) {
	case int:
		n = x
	case *int:
		if x == nil {
			return nil
		}
		n = *x
	}
	if n == 0 {
		return nil
	}
	s := time.Unix(int64(n), 0).Format(time.RFC3339)
	return &s
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func anyToIntPtr(v interface{}) *int {
	switch x := v.(type) {
	case float64:
		n := int(x)
		return &n
	case int:
		return &x
	}
	return nil
}

var unsafeFileChars = regexp.MustCompile(`[^\p{L}\p{N}._ -]+`)

func sanitizeFileName(name string) string {
	// keep the whole name (Kanboard allows "/" in names), just make it safe
	name = unsafeFileChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, ". ")
	if name == "" {
		return "file"
	}
	return name
}
