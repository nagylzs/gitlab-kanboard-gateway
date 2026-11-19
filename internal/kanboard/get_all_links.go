package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetAllLinks returns the relation types of internal links ("relates to", "blocks", ...).
func GetAllLinks(kbCfg config.KanboardConfig) ([]KbResponseLink, error) {
	// https://docs.kanboard.org/v1/api/link_procedures/#getalllinks
	var resp KbResponse[[]KbResponseLink]
	err := WebClientRpcCall(kbCfg, newRequest("getAllLinks", struct{}{}), &resp)
	return resp.Result, err
}
