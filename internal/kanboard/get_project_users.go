package kanboard

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// GetProjectUsers returns user id -> display name (name, or username when the name is
// empty) of the project members, including members through groups. Unlike getUser it
// is allowed for every project member, not only for admins.
func GetProjectUsers(kbCfg config.KanboardConfig, projectId int) (map[int]string, error) {
	// https://docs.kanboard.org/v1/api/project_permission_procedures/#getprojectusers
	var resp KbResponse[json.RawMessage]
	err := WebClientRpcCall(kbCfg, newRequest("getProjectUsers", KbProjectIdParam{ProjectId: int32(projectId)}), &resp)
	if err != nil {
		return nil, err
	}
	users, err := decodeProjectUsers(resp.Result)
	if err != nil {
		return nil, fmt.Errorf("getProjectUsers: %w", err)
	}
	return users, nil
}

// decodeProjectUsers skips entries that are not a user id with a name. The live server
// returns `"": null` for a group member whose user no longer exists, and a project
// without members may come back as [].
func decodeProjectUsers(raw json.RawMessage) (map[int]string, error) {
	users := map[int]string{}
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return users, nil
	}
	var m map[string]*string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	for k, v := range m {
		id, err := strconv.Atoi(k)
		if err != nil || v == nil {
			slog.Warn("getProjectUsers: skipping invalid entry", "key", k, "value", v)
			continue
		}
		users[id] = *v
	}
	return users, nil
}
