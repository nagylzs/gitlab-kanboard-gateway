package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

type KbSearchTasksParam struct {
	ProjectId int    `json:"project_id"`
	Query     string `json:"query"`
}

// KbResponseSearchTask is one row of searchTasks. It is a task row joined with the
// names of its project, column, swimlane, category and assignee, plus counters.
type KbResponseSearchTask struct {
	Id                  int     `json:"id"`
	Title               string  `json:"title"`
	Description         string  `json:"description"`
	Reference           string  `json:"reference"`
	IsActive            bool    `json:"is_active"`
	ProjectId           int     `json:"project_id"`
	ProjectName         string  `json:"project_name"`
	ColumnId            int     `json:"column_id"`
	ColumnName          string  `json:"column_name"`
	SwimlaneId          int     `json:"swimlane_id"`
	SwimlaneName        *string `json:"swimlane_name"`
	CategoryId          int     `json:"category_id"`
	CategoryName        *string `json:"category_name"`
	OwnerId             int     `json:"owner_id"`
	AssigneeUsername    *string `json:"assignee_username"`
	AssigneeName        *string `json:"assignee_name"`
	CreatorId           int     `json:"creator_id"`
	ColorId             string  `json:"color_id"`
	Priority            int     `json:"priority"`
	Score               float64 `json:"score"`
	DateCreation        *int    `json:"date_creation"`
	DateModification    *int    `json:"date_modification"`
	DateCompleted       *int    `json:"date_completed"`
	DateStarted         *int    `json:"date_started"`
	DateDue             *int    `json:"date_due"`
	DateMoved           *int    `json:"date_moved"`
	TimeSpent           float64 `json:"time_spent"`
	TimeEstimated       float64 `json:"time_estimated"`
	NbComments          int     `json:"nb_comments"`
	NbFiles             int     `json:"nb_files"`
	NbSubtasks          int     `json:"nb_subtasks"`
	NbCompletedSubtasks int     `json:"nb_completed_subtasks"`
	NbLinks             int     `json:"nb_links"`
	NbExternalLinks     int     `json:"nb_external_links"`
}

// SearchTasks runs a Kanboard search query (same syntax as the web UI search box)
// inside one project.
func SearchTasks(kbCfg config.KanboardConfig, projectId int, query string) ([]KbResponseSearchTask, error) {
	// https://docs.kanboard.org/v1/api/task_procedures/#searchtasks
	var resp KbResponse[[]KbResponseSearchTask]
	err := WebClientRpcCall(kbCfg, newRequest("searchTasks", KbSearchTasksParam{ProjectId: projectId, Query: query}), &resp)
	return resp.Result, err
}
