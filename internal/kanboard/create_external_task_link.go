package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// CreateExternalTaskLink attaches a URL to a task and returns the id of the link. The
// link type is detected by Kanboard ("auto": web link, or attachment for file URLs);
// every built-in type has only the "related" dependency. With an empty title Kanboard
// fetches the page and uses its title.
func CreateExternalTaskLink(kbCfg config.KanboardConfig, taskId int, url, title string) (int, error) {
	// https://docs.kanboard.org/v1/api/external_task_link_procedures/#createexternaltasklink
	return writeId(kbCfg, "createExternalTaskLink", struct {
		TaskId     int    `json:"task_id"`
		Url        string `json:"url"`
		Dependency string `json:"dependency"`
		Type       string `json:"type"`
		Title      string `json:"title,omitempty"`
	}{taskId, url, "related", "auto", title})
}
