package kanboard

// Result types for the read-only task detail procedures.
// Field types follow what Kanboard >= 1.2.5x actually returns (ints/bools),
// not the string-typed examples in the API documentation.

// https://docs.kanboard.org/v1/api/comment_procedures/#getallcomments
type KbResponseComment struct {
	Id               int     `json:"id"`
	DateCreation     int     `json:"date_creation"`
	DateModification int     `json:"date_modification"`
	TaskId           int     `json:"task_id"`
	UserId           int     `json:"user_id"`
	Comment          string  `json:"comment"` // markdown
	Visibility       string  `json:"visibility"`
	Username         string  `json:"username"`
	Name             string  `json:"name"`
	Email            string  `json:"email"`
	AvatarPath       *string `json:"avatar_path"`
}

// https://docs.kanboard.org/v1/api/task_file_procedures/#getalltaskfiles
type KbResponseTaskFile struct {
	Id       int     `json:"id"`
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	IsImage  bool    `json:"is_image"`
	TaskId   int     `json:"task_id"`
	Date     int     `json:"date"`
	UserId   int     `json:"user_id"`
	Size     int64   `json:"size"`
	Username *string `json:"username"`
	UserName *string `json:"user_name"`
	Etag     string  `json:"etag"`
}

// https://docs.kanboard.org/v1/api/subtask_procedures/#getallsubtasks
type KbResponseSubtask struct {
	Id            int     `json:"id"`
	Title         string  `json:"title"`
	Status        int     `json:"status"` // 0=todo 1=in progress 2=done
	TimeEstimated float64 `json:"time_estimated"`
	TimeSpent     float64 `json:"time_spent"`
	TaskId        int     `json:"task_id"`
	UserId        int     `json:"user_id"`
	Username      *string `json:"username"`
	Name          *string `json:"name"`
	StatusName    string  `json:"status_name"` // localized by the server
}

// https://docs.kanboard.org/v1/api/internal_task_link_procedures/#getalltasklinks
type KbResponseTaskLink struct {
	Id                   int     `json:"id"`
	TaskId               int     `json:"task_id"` // the *other* task
	Label                string  `json:"label"`   // e.g. "relates to", "blocks"
	Title                string  `json:"title"`
	IsActive             bool    `json:"is_active"`
	ProjectId            int     `json:"project_id"`
	ProjectName          string  `json:"project_name"`
	ColumnId             int     `json:"column_id"`
	ColumnTitle          string  `json:"column_title"`
	ColorId              string  `json:"color_id"`
	DateCompleted        *int    `json:"date_completed"`
	DateStarted          *int    `json:"date_started"`
	DateDue              *int    `json:"date_due"`
	TaskTimeSpent        float64 `json:"task_time_spent"`
	TaskTimeEstimated    float64 `json:"task_time_estimated"`
	TaskAssigneeId       int     `json:"task_assignee_id"`
	TaskAssigneeUsername *string `json:"task_assignee_username"`
	TaskAssigneeName     *string `json:"task_assignee_name"`
}

// https://docs.kanboard.org/v1/api/external_task_link_procedures/#getallexternaltasklinks
type KbResponseExternalTaskLink struct {
	Id               int     `json:"id"`
	LinkType         string  `json:"link_type"` // weblink, attachment, ...
	Dependency       string  `json:"dependency"`
	Title            string  `json:"title"`
	Url              string  `json:"url"`
	DateCreation     int     `json:"date_creation"`
	DateModification int     `json:"date_modification"`
	TaskId           int     `json:"task_id"`
	CreatorId        int     `json:"creator_id"`
	CreatorName      *string `json:"creator_name"`
	CreatorUsername  *string `json:"creator_username"`
	DependencyLabel  string  `json:"dependency_label"` // localized
	Type             string  `json:"type"`             // localized
}

// https://docs.kanboard.org/v1/api/user_procedures/#getuser
// Only the fields we need; the password hash is deliberately not decoded.
type KbResponseUser struct {
	Id       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	IsActive bool   `json:"is_active"`
}
