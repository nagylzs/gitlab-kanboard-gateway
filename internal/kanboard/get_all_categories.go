package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetAllCategories returns the categories of a project.
func GetAllCategories(kbCfg config.KanboardConfig, projectId int) ([]KbResponseCategory, error) {
	// https://docs.kanboard.org/v1/api/category_procedures/#getallcategories
	var resp KbResponse[[]KbResponseCategory]
	err := WebClientRpcCall(kbCfg, newRequest("getAllCategories", KbProjectIdParam{ProjectId: int32(projectId)}), &resp)
	return resp.Result, err
}
