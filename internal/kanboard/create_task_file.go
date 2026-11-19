package kanboard

import (
	"encoding/base64"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// CreateTaskFile uploads an attachment and returns its id. Kanboard makes a thumbnail
// of image files (by their name); an image that cannot be decoded crashes the
// procedure, which then answers with an HTML error page instead of JSON.
func CreateTaskFile(kbCfg config.KanboardConfig, projectId, taskId int, filename string, data []byte) (int, error) {
	// https://docs.kanboard.org/v1/api/task_file_procedures/#createtaskfile
	return writeId(kbCfg, "createTaskFile", struct {
		ProjectId int    `json:"project_id"`
		TaskId    int    `json:"task_id"`
		Filename  string `json:"filename"`
		Blob      string `json:"blob"`
	}{projectId, taskId, filename, base64.StdEncoding.EncodeToString(data)})
}
