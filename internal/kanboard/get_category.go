package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetCategory returns nil (and no error) when the category does not exist (e.g. id 0).
func GetCategory(kbCfg config.KanboardConfig, categoryId int) (*KbResponseCategory, error) {
	// https://docs.kanboard.org/v1/api/category_procedures/#getcategory
	var resp KbResponse[*KbResponseCategory]
	err := WebClientRpcCall(kbCfg, newRequest("getCategory", KbCategoryIdParam{CategoryId: categoryId}), &resp)
	return resp.Result, err
}
