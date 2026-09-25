package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetSwimlane returns nil (and no error) when the swimlane does not exist.
func GetSwimlane(kbCfg config.KanboardConfig, swimlaneId int) (*KbResponseSwimlane, error) {
	// https://docs.kanboard.org/v1/api/swimlane_procedures/#getswimlane
	var resp KbResponse[*KbResponseSwimlane]
	err := WebClientRpcCall(kbCfg, newRequest("getSwimlane", KbSwimlaneIdParam{SwimlaneId: swimlaneId}), &resp)
	return resp.Result, err
}
