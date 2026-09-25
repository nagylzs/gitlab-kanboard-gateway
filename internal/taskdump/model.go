// Package taskdump builds a self-contained, human/AI readable JSON document
// about a single Kanboard task by combining several read-only API calls.
//
// Design goals for the output (consumed by AI agents):
//   - every id that Kanboard returns is resolved to a name where possible
//     (project, column, swimlane, category, users)
//   - timestamps are RFC3339 strings, 0/null becomes null
//   - enumerations are plain lowercase words ("open"/"closed", "todo"/"done")
//   - markdown bodies (description, comments) are passed through untouched
//   - nothing is ever written to Kanboard
package taskdump

// TaskDocument is the top-level JSON object printed for each task.
type TaskDocument struct {
	Id          int    `json:"id"`
	Url         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"` // markdown
	Status      string `json:"status"`      // "open" | "closed"

	Project  ProjectRef `json:"project"`
	Column   *NamedRef  `json:"column"`   // null if the column could not be resolved
	Swimlane *NamedRef  `json:"swimlane"` // null if no swimlane / not resolvable
	Category *NamedRef  `json:"category"` // null if the task has no category

	Color     string `json:"color"`
	Priority  int    `json:"priority"`
	Score     int    `json:"score"`
	Reference string `json:"reference"` // free-form external reference field
	Position  int    `json:"position"`  // position inside the column

	Owner   *UserRef `json:"owner"`   // assignee, null if unassigned
	Creator *UserRef `json:"creator"` // null if unknown

	Dates        Dates             `json:"dates"`
	TimeTracking TimeTracking      `json:"time_tracking"`
	Tags         []string          `json:"tags"`
	Metadata     map[string]string `json:"metadata"`
	Recurrence   *Recurrence       `json:"recurrence,omitempty"`
	External     *ExternalTask     `json:"external_task,omitempty"`

	Subtasks      []Subtask      `json:"subtasks"`
	Comments      []Comment      `json:"comments"`
	Attachments   []Attachment   `json:"attachments"`
	Links         []Link         `json:"links"`          // internal task-to-task links
	ExternalLinks []ExternalLink `json:"external_links"` // URLs attached to the task

	// Non-fatal problems encountered while collecting the sub-resources
	// (e.g. a failed API call); the rest of the document is still valid.
	Warnings []string `json:"warnings,omitempty"`

	FetchedAt string `json:"fetched_at"`
}

type ProjectRef struct {
	Id         int    `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier,omitempty"`
	BoardUrl   string `json:"board_url,omitempty"`
}

type NamedRef struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type UserRef struct {
	Id       int    `json:"id"`
	Username string `json:"username,omitempty"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
}

type Dates struct {
	Created   *string `json:"created"`
	Modified  *string `json:"modified"`
	Moved     *string `json:"moved"` // last moved between columns
	Started   *string `json:"started"`
	Due       *string `json:"due"`
	Completed *string `json:"completed"`
}

type TimeTracking struct {
	SpentHours     float64 `json:"spent_hours"`
	EstimatedHours float64 `json:"estimated_hours"`
}

type Recurrence struct {
	Status    int  `json:"status"`
	Trigger   int  `json:"trigger"`
	Factor    int  `json:"factor"`
	Timeframe int  `json:"timeframe"`
	Basedate  int  `json:"basedate"`
	ParentId  *int `json:"parent_task_id"`
	ChildId   *int `json:"child_task_id"`
}

type ExternalTask struct {
	Provider string `json:"provider"`
	Uri      string `json:"uri"`
}

type Subtask struct {
	Id           int          `json:"id"`
	Title        string       `json:"title"`
	Status       string       `json:"status"` // "todo" | "in_progress" | "done"
	Assignee     *UserRef     `json:"assignee"`
	TimeTracking TimeTracking `json:"time_tracking"`
}

type Comment struct {
	Id         int     `json:"id"`
	Created    *string `json:"created"`
	Modified   *string `json:"modified"`
	Author     UserRef `json:"author"`
	Visibility string  `json:"visibility,omitempty"`
	// Markdown. Images pasted into the comment (which Kanboard stores as task
	// attachments and embeds as relative <img> tags) are rewritten to
	// ![<attachment name>](<local_path or absolute url>).
	Content string `json:"content"`
	// Ids of entries in the task's "attachments" list that this comment embeds.
	AttachmentIds []int `json:"attachment_ids,omitempty"`
}

type Attachment struct {
	Id         int      `json:"id"`
	Name       string   `json:"name"`
	SizeBytes  int64    `json:"size_bytes"`
	IsImage    bool     `json:"is_image"`
	Uploaded   *string  `json:"uploaded"`
	UploadedBy *UserRef `json:"uploaded_by"`
	// Absolute path of the downloaded copy; only present when --download-dir was given
	// and the download succeeded.
	LocalPath string `json:"local_path,omitempty"`
	// Why the file was not downloaded (too large, API error); absent on success or
	// when downloading was not requested.
	DownloadError string `json:"download_error,omitempty"`
}

// Link is an internal Kanboard link between this task and another task.
type Link struct {
	Id       int        `json:"id"`
	Relation string     `json:"relation"` // e.g. "relates to", "blocks", "is blocked by"
	Task     LinkedTask `json:"task"`
}

type LinkedTask struct {
	Id           int          `json:"id"`
	Title        string       `json:"title"`
	Status       string       `json:"status"`
	Project      NamedRef     `json:"project"`
	Column       string       `json:"column"`
	Assignee     *UserRef     `json:"assignee"`
	Dates        LinkedDates  `json:"dates"`
	TimeTracking TimeTracking `json:"time_tracking"`
}

type LinkedDates struct {
	Started   *string `json:"started"`
	Due       *string `json:"due"`
	Completed *string `json:"completed"`
}

type ExternalLink struct {
	Id       int      `json:"id"`
	Type     string   `json:"type"`     // "weblink", "attachment", ...
	Relation string   `json:"relation"` // "related", ...
	Title    string   `json:"title"`
	Url      string   `json:"url"`
	Created  *string  `json:"created"`
	Modified *string  `json:"modified"`
	Creator  *UserRef `json:"creator"`
}
