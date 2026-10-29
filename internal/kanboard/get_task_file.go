package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetTaskFile returns the metadata of one attachment; nil (and no error) when the file
// does not exist. Unlike getAllTaskFiles, the uploader's names are not included.
func GetTaskFile(kbCfg config.KanboardConfig, fileId int) (*KbResponseTaskFile, error) {
	// https://docs.kanboard.org/v1/api/task_file_procedures/#gettaskfile
	var resp KbResponse[*KbResponseTaskFile]
	err := WebClientRpcCall(kbCfg, newRequest("getTaskFile", KbFileIdParam{FileId: fileId}), &resp)
	return resp.Result, err
}
