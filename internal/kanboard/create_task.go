package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// KbCreateTaskParams are the createTask parameters this program uses. Zero values are
// left out, so Kanboard uses its defaults (first column, default swimlane, ...).
type KbCreateTaskParams struct {
	Title       string   `json:"title"`
	ProjectId   int      `json:"project_id"`
	Description string   `json:"description,omitempty"`
	ColumnId    int      `json:"column_id,omitempty"`
	SwimlaneId  int      `json:"swimlane_id,omitempty"`
	CategoryId  int      `json:"category_id,omitempty"`
	OwnerId     int      `json:"owner_id,omitempty"`
	CreatorId   int      `json:"creator_id,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// CreateTask returns the id of the new task. Kanboard refuses (false) an owner or
// creator that is not a member of the project; unknown tags are created.
func CreateTask(kbCfg config.KanboardConfig, params KbCreateTaskParams) (int, error) {
	// https://docs.kanboard.org/v1/api/task_procedures/#createtask
	return writeId(kbCfg, "createTask", params)
}
