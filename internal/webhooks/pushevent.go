package webhooks

type PushEvent struct {
	ObjectKind        string   `json:"object_kind"` // push
	EventName         string   `json:"event_name"`  // push
	Before            string   `json:"before"`      //3b248ae2e277416bad2a8527af00c371bd07026a
	After             string   `json:"after"`       //dd91c92cccdef54dd84d6d8e6b27d4fcba28612c
	Ref               string   `json:"ref"`         // "refs/heads/master"
	RefProtected      bool     `json:"ref_protected"`
	CheckoutSha       string   `json:"checkout_sha"`  // dd91c92cccdef54dd84d6d8e6b27d4fcba28612c
	Message           string   `json:"message"`       // may be null
	UserId            int      `json:"user_id"`       // 3
	UserName          string   `json:"user_name"`     // "Nagy László Zsolt"
	UserUserName      string   `json:"user_username"` // nagy.laszlo.zsolt
	UserEmail         string   `json:"user_email"`    // may be empty
	UserAvatar        string   `json:"user_avatar"`   // "https://your_gitlab.com/uploads/-/system/user/avatar/3/avatar.png"
	ProjectId         int      `json:"project_id"`
	Project           Project  `json:"project"`
	Commits           []Commit `json:"commits"`
	TotalCommitsCount int      `json:"total_commits_count"`
	// PushOptions interface{} `json:"push_options"`
	Repository Repository `json:"repository"`

	CanRetry bool // not returned by gitlab, this is a flag used internally
}

type Project struct {
	Id                int    `json:"id"`
	Name              string `json:"name"`        // your-project
	Description       string `json:"description"` // "your-project description"
	WebUrl            string `json:"web_url"`     // "https://your_gitlab.com/team_name/project_name"
	AvatarUrl         string `json:"avatar_url"`
	GitSshUrl         string `json:"git_ssh_url"`  // "git@your_gitlab.com:team_name/repo_name.git"
	GitHttpUrl        string `json:"git_http_url"` // "https://your_gitlab.com/team_name/repo_name.git"
	Namespace         string `json:"namespace"`    // "team_name"
	VisibilityLevel   int    `json:"visibility_level"`
	PathWithNamespace string `json:"path_with_namespace"` // "team_name/project_name"
	DefaultBranch     string `json:"default_branch"`      // "master"
	CiConfigPath      string `json:"ci_config_path"`      // may be null
	Homepage          string `json:"homepage"`            // "https://your_gitlab.com/team_name/project_name"
	Url               string `json:"url"`                 // "git@your_gitlab.com:team_name/project_name.git"
	SshUrl            string `json:"ssh_url"`             // "git@your_gitlab.com:team_name/project_name.git"
	HttpUrl           string `json:"http_url"`            // "https://your_gitlab.com/team_name/project_name.git"
}

type Commit struct {
	Id        string   `json:"id"`        // "dd91c92cccdef54dd84d6d8e6b27d4fcba28612c"
	Message   string   `json:"message"`   // "gyker.gyker_notification fv. fix - https://your_kanboard.com/task/12222\n"
	Title     string   `json:"title"`     // "gyker.gyker_notification fv. fix - https://your_kanboard.com/task/12222"
	Timestamp string   `json:"timestamp"` // "2024-06-21T09:51:21+02:00"
	Url       string   `json:"url"`       // "https://your_gitlab.com/team_name/project_name/-/commit/dd91c92cccdef54dd84d6d8e6b27d4fcba28612c"
	Author    Author   `json:"author"`
	Added     []string `json:"added"`    // a list of file names
	Modified  []string `json:"modified"` // a list of file names
	Removed   []string `json:"removed"`  // a list of file names
}

type Repository struct {
	Name            string `json:"name"`         // "project_name"
	Url             string `json:"url"`          // "git@your_gitlab.com:team_name/project_name.git"
	Description     string `json:"description"`  // "project_name description."
	Homepage        string `json:"homepage"`     // "https://your_gitlab.com/team_name/project_name"
	GitHttpUrl      string `json:"git_http_url"` // "https://your_gitlab.com/team_name/project_name.git"
	GitSShUrl       string `json:"git_ssh_url"`  // "git@your_gitlab.com:team_name/project_name.git
	VisibilityLevel int    `json:"visibility_level"`
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}
