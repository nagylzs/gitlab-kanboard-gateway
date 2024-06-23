package kanboard

type KbResponseTask struct {
	Id                  int         `json:"id"`
	Title               string      `json:"title"`
	Description         string      `json:"description"`
	DateCreation        *int        `json:"date_creation"`
	ColorId             string      `json:"color_id"`
	ProjectId           int         `json:"project_id"`
	ColumnId            int         `json:"column_id"`
	OwnerId             int         `json:"owner_id"`
	Position            int         `json:"position"`
	IsActive            bool        `json:"is_active"`
	DateCompleted       *int        `json:"date_completed"`
	Score               float64     `json:"score"`
	DateDue             *int        `json:"date_due"`
	CategoryId          int         `json:"category_id"`
	CreatorId           int         `json:"creator_id"`
	DateModification    *int        `json:"date_modification"`
	Reference           string      `json:"reference"`
	DateStarted         *int        `json:"date_started"`
	TimeSpent           string      `json:"time_spent"`
	TimeEstimated       string      `json:"time_estimated"`
	SwimlaneId          int         `json:"swimlane_id"`
	DateMoved           *int        `json:"date_moved"`
	RecurrenceStatus    int         `json:"recurrence_status"`
	RecurrenceTrigger   int         `json:"recurrence_trigger"`
	RecurrenceFactor    int         `json:"recurrence_factor"`
	RecurrenceTimeframe int         `json:"recurrence_timeframe"`
	RecurrenceBasedate  *int        `json:"recurrence_basedate"`
	RecurrenceParent    interface{} `json:"recurrence_parent"`
	RecurrenceChild     interface{} `json:"recurrence_child"`
	Priority            int         `json:"priority"`
	ExternalProvider    interface{} `json:"external_provider"`
	ExternalUri         interface{} `json:"external_uri"`
	Url                 string      `json:"url"`
	Color               struct {
		Name       string `json:"name"`
		Background string `json:"background"`
		Border     string `json:"border"`
	} `json:"color"`
}
