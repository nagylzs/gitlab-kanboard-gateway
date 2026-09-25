package kanboard

import (
	"encoding/json"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// GetTaskMetadata returns the custom key/value metadata attached to the task.
func GetTaskMetadata(kbCfg config.KanboardConfig, taskId int) (map[string]string, error) {
	// https://docs.kanboard.org/v1/api/task_metadata_procedures/#gettaskmetadata
	var resp KbResponse[json.RawMessage]
	err := WebClientRpcCall(kbCfg, newRequest("getTaskMetadata", KbTaskIdParam{TaskId: taskId}), &resp)
	if err != nil {
		return nil, err
	}
	return decodeStringMap(resp.Result)
}

// decodeStringMap accepts {}, {"k":"v"}, [], [{"k":"v"}] and null.
func decodeStringMap(raw json.RawMessage) (map[string]string, error) {
	res := make(map[string]string)
	if len(raw) == 0 || string(raw) == "null" {
		return res, nil
	}
	if raw[0] == '{' {
		err := json.Unmarshal(raw, &res)
		return res, err
	}
	var list []map[string]string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	for _, m := range list {
		for k, v := range m {
			res[k] = v
		}
	}
	return res, nil
}
