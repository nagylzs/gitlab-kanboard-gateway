package kanboard

import (
	"encoding/json"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// GetTaskTags returns tag id -> tag name.
func GetTaskTags(kbCfg config.KanboardConfig, taskId int) (map[string]string, error) {
	// https://docs.kanboard.org/v1/api/tags_procedures/#gettasktags
	// PHP serializes an empty associative array as [] and a non-empty one as {},
	// so decode via RawMessage.
	var resp KbResponse[json.RawMessage]
	err := WebClientRpcCall(kbCfg, newRequest("getTaskTags", KbTaskIdParam{TaskId: taskId}), &resp)
	if err != nil {
		return nil, err
	}
	return decodeStringMap(resp.Result)
}
