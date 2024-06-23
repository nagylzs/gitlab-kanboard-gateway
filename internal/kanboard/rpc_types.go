package kanboard

type KBResponseErrorBody struct {
	Code    int32  `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

type KBResponseError struct {
	JsonRpc string              `json:"jsonrpc"`
	Id      int32               `json:"id"`
	Error   KBResponseErrorBody `json:"error"`
}

type KbAccountLevelRequest struct {
	JsonRpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Id      int32  `json:"id"`
}

type KbProjectLevelRequest struct {
	JsonRpc string           `json:"jsonrpc"`
	Method  string           `json:"method"`
	Id      int32            `json:"id"`
	Params  KbProjectIdParam `json:"params"`
}

type KbProjectIdParam struct {
	ProjectId int32 `json:"project_id"`
}

type KbResponseGetAllProjects struct {
	JsonRpc string              `json:"jsonrpc"`
	Id      int32               `json:"id"`
	Result  []KbResponseProject `json:"result"`
}

type KbResponseProject struct {
	Id                  int32                 `json:"id"`
	Name                string                `json:"name"`
	IsActive            bool                  `json:"is_active"`
	Token               string                `json:"token"`         // empty
	LastModified        int32                 `json:"last_modified"` // ticks
	IsPublic            bool                  `json:"is_public"`
	IsPrivate           bool                  `json:"is_private"`
	DefaultSwimlane     string                `json:"default_swimlane"`
	ShowDefaultSwimlane bool                  `json:"show_default_swimlane"`
	Description         string                `json:"description"` // may be empty
	Identifier          string                `json:"identifier"`  // may be empty
	Url                 KbResponseProjectUrls `json:"url"`
}

type KbResponseProjectUrls struct {
	Board    string `json:"board"`
	Calendar string `json:"calendar"`
	List     string `json:"list"`
}

type KbResponseGetColorList struct {
	JsonRpc string            `json:"jsonrpc"`
	Id      int32             `json:"id"`
	Colors  map[string]string `json:"result"`
}
type KbResponseGetAllCategories struct {
	JsonRpc string               `json:"jsonrpc"`
	Id      int32                `json:"id"`
	Result  []KbResponseCategory `json:"result"`
}

type KbResponseCategory struct {
	Id          int32  `json:"id"`
	Name        string `json:"name"`
	ProjectId   int32  `json:"project_id"`
	Description string `json:"description"`
	ColorId     string `json:"color_id"`
}

type KbResponseGetAllSwimlanes struct {
	JsonRpc string               `json:"jsonrpc"`
	Id      int32                `json:"id"`
	Result  []KbResponseSwimlane `json:"result"`
}

type KbResponseSwimlane struct {
	Id          int32  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ProjectId   int32  `json:"project_id"`
	Position    int32  `json:"position"`
	IsActive    bool   `json:"bool"`
}

type KbResponseGetColumns struct {
	JsonRpc string             `json:"jsonrpc"`
	Id      int32              `json:"id"`
	Result  []KbResponseColumn `json:"result"`
}

type KbResponseColumn struct {
	Id        int32  `json:"id"`
	Title     string `json:"title"`
	ProjectId int32  `json:"project_id"`
	Position  int32  `json:"position"`
	TaskLimit int32  `json:"task_limit"`
}

type KbResponseGetTags struct {
	JsonRpc string          `json:"jsonrpc"`
	Id      int32           `json:"id"`
	Result  []KbResponseTag `json:"result"`
}

type KbResponseTag struct {
	Id        int32  `json:"id"`
	Name      string `json:"name"`
	ProjectId int32  `json:"project_id"`
	ColorId   string `json:"color_id"` // Undocumented field, "None" means no color :-(
}

type KbResponseGetAllTasks struct {
	JsonRpc string           `json:"jsonrpc"`
	Id      int32            `json:"id"`
	Result  []KbResponseTask `json:"result"`
}

type KbCreateCommentRequest struct {
	JsonRpc string                `json:"jsonrpc"`
	Method  string                `json:"method"`
	Id      int                   `json:"id"`
	Params  KbCreateCommentParams `json:"params"`
}

type KbCreateCommentParams struct {
	TaskId  int    `json:"task_id"`
	UserId  int    `json:"user_id"`
	Content string `json:"content"`
}

type KbCreateCommentResponse struct {
	JsonRpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Result  int    `json:"result"`
}
