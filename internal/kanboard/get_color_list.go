package kanboard

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// KbColor is a task color: Id is what a task stores in color_id, Name is what the web
// UI shows, translated to the server's language.
type KbColor struct {
	Id   string
	Name string
}

// KbColorProperties are the CSS colors of one of Kanboard's built-in task colors.
type KbColorProperties struct {
	Name       string `json:"name"` // English, not translated
	Background string `json:"background"`
	Border     string `json:"border"`
}

// GetColorList returns the colors a task can have, in the order of the web UI's color
// menu. Plugins can add colors to it; createTask and updateTask accept exactly these ids.
func GetColorList(kbCfg config.KanboardConfig) ([]KbColor, error) {
	// https://docs.kanboard.org/v1/api/application_procedures/#getcolorlist
	var resp KbResponse[json.RawMessage]
	if err := WebClientRpcCall(kbCfg, KbAccountLevelRequest{JsonRpc: "2.0", Method: "getColorList", Id: 1}, &resp); err != nil {
		return nil, err
	}
	return decodeColorList(resp.Result)
}

// decodeColorList decodes a {"id": "name", ...} object in its order; a map would lose
// it. PHP encodes an empty list as [].
func decodeColorList(data json.RawMessage) ([]KbColor, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" || string(data) == "[]" {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("getColorList: expected a JSON object, got %.40s", data)
	}
	var res []KbColor
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("getColorList: %w", err)
		}
		id, _ := tok.(string)
		var name string
		if err := dec.Decode(&name); err != nil {
			return nil, fmt.Errorf("getColorList: color %q: %w", id, err)
		}
		res = append(res, KbColor{Id: id, Name: name})
	}
	return res, nil
}

// GetDefaultTaskColors returns Kanboard's built-in colors with their CSS colors.
// Colors added by a plugin are not among them.
func GetDefaultTaskColors(kbCfg config.KanboardConfig) (map[string]KbColorProperties, error) {
	// https://docs.kanboard.org/v1/api/application_procedures/#getdefaulttaskcolors
	var resp KbResponse[json.RawMessage]
	if err := WebClientRpcCall(kbCfg, KbAccountLevelRequest{JsonRpc: "2.0", Method: "getDefaultTaskColors", Id: 1}, &resp); err != nil {
		return nil, err
	}
	res := make(map[string]KbColorProperties)
	if data := bytes.TrimSpace(resp.Result); len(data) == 0 || string(data) == "null" || string(data) == "[]" {
		return res, nil
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("getDefaultTaskColors: %w", err)
	}
	return res, nil
}

// GetDefaultTaskColor returns the color id that a new task gets when it is created
// without a color.
func GetDefaultTaskColor(kbCfg config.KanboardConfig) (string, error) {
	// https://docs.kanboard.org/v1/api/application_procedures/#getdefaulttaskcolor
	var resp KbResponse[string]
	err := WebClientRpcCall(kbCfg, KbAccountLevelRequest{JsonRpc: "2.0", Method: "getDefaultTaskColor", Id: 1}, &resp)
	return resp.Result, err
}
