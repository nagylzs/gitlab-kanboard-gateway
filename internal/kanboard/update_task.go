package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

type KbUpdateTaskOwnerParams struct {
	Id      int `json:"id"`
	OwnerId int `json:"owner_id"` // 0 = nobody
}

// UpdateTaskOwner changes only the assignee of a task. Kanboard refuses (false) a
// user who cannot be assigned in the task's project.
func UpdateTaskOwner(kbCfg config.KanboardConfig, taskId, ownerId int) error {
	// https://docs.kanboard.org/v1/api/task_procedures/#updatetask
	return writeOk(kbCfg, "updateTask", KbUpdateTaskOwnerParams{Id: taskId, OwnerId: ownerId})
}

type KbUpdateTaskColorParams struct {
	Id      int    `json:"id"`
	ColorId string `json:"color_id"`
}

// UpdateTaskColor changes only the color of a task. Kanboard refuses (false) a color
// id that is not in GetColorList.
func UpdateTaskColor(kbCfg config.KanboardConfig, taskId int, colorId string) error {
	// https://docs.kanboard.org/v1/api/task_procedures/#updatetask
	return writeOk(kbCfg, "updateTask", KbUpdateTaskColorParams{Id: taskId, ColorId: colorId})
}
