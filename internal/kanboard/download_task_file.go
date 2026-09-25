package kanboard

import (
	"encoding/base64"
	"fmt"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// DownloadTaskFile returns the decoded file content. Kanboard answers with an
// empty string for unknown file ids, which is reported as an error here.
func DownloadTaskFile(kbCfg config.KanboardConfig, fileId int) ([]byte, error) {
	// https://docs.kanboard.org/v1/api/task_file_procedures/#downloadtaskfile
	var resp KbResponse[string]
	err := WebClientRpcCall(kbCfg, newRequest("downloadTaskFile", KbFileIdParam{FileId: fileId}), &resp)
	if err != nil {
		return nil, err
	}
	if resp.Result == "" {
		return nil, fmt.Errorf("file %v not found or empty", fileId)
	}
	return base64.StdEncoding.DecodeString(resp.Result)
}
