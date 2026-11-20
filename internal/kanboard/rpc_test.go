package kanboard

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

func TestRpcAnswerAfterPhpNotice(t *testing.T) {
	cases := []struct {
		name, body string
		wantId     int
		wantErr    string
	}{
		{"plain", `{"jsonrpc":"2.0","result":41,"id":1}`, 41, ""},
		// What the official Kanboard image prints when a plugin triggers a deprecation warning.
		{"notice", "<br />\n<b>Deprecated</b>:  Implicitly marking parameter $x as nullable is deprecated in " +
			"<b>/var/www/app/plugins/X/Y.php</b> on line <b>270</b><br />\n" + `{"jsonrpc":"2.0","result":42,"id":1}`, 42, ""},
		{"error page", "<html><body><h1>502 Bad Gateway</h1></body></html>", 0, "without JSON: 502 Bad Gateway"},
		{"truncated", `<b>Warning</b> {"jsonrpc":"2.0","result":`, 0, "without JSON"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, c.body)
			}))
			defer srv.Close()
			id, err := CreateTask(config.KanboardConfig{ApiUrl: srv.URL}, KbCreateTaskParams{Title: "x", ProjectId: 1})
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Errorf("want an error with %q, got id %v, error %v", c.wantErr, id, err)
				}
				return
			}
			if err != nil || id != c.wantId {
				t.Errorf("got id %v, error %v; want id %v", id, err, c.wantId)
			}
		})
	}
}
