package kanboard

import (
	"encoding/json"
	"fmt"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// ErrRefused means that Kanboard answered a write procedure with false (or 0): it
// checked the request and did not do it, without telling why. Typical reasons are an
// id that does not exist or does not belong to the project, a user who is not a
// member of the project, or (with a personal token) a change in another user's name.
type ErrRefused struct {
	Method string
}

func (e ErrRefused) Error() string {
	return fmt.Sprintf("%v: Kanboard refused the change (it returned false without a reason)", e.Method)
}

// writeId calls a procedure whose result is the id of the new object, or false.
func writeId[P any](kbCfg config.KanboardConfig, method string, params P) (int, error) {
	var resp KbResponse[json.RawMessage]
	if err := WebClientRpcCall(kbCfg, newRequest(method, params), &resp); err != nil {
		return 0, err
	}
	var id int
	if json.Unmarshal(resp.Result, &id) != nil || id <= 0 {
		return 0, ErrRefused{Method: method}
	}
	return id, nil
}

// writeOk calls a procedure whose result is true on success.
func writeOk[P any](kbCfg config.KanboardConfig, method string, params P) error {
	var resp KbResponse[json.RawMessage]
	if err := WebClientRpcCall(kbCfg, newRequest(method, params), &resp); err != nil {
		return err
	}
	var ok bool
	if json.Unmarshal(resp.Result, &ok) != nil || !ok {
		return ErrRefused{Method: method}
	}
	return nil
}
