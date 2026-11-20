package kanboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// RpcError is an error object returned by the Kanboard JSON-RPC server.
type RpcError struct {
	HttpStatus int
	Code       int32
	Message    string
	Data       string
}

func (e *RpcError) Error() string {
	return fmt.Sprintf("error httpStatus=%v errorCode=%v errorMessage=%v data=%v",
		e.HttpStatus, e.Code, e.Message, e.Data)
}

// IsForbidden reports whether the server refused the procedure for the logged-in user:
// an admin-only procedure, or a project the user is not a member of.
func IsForbidden(err error) bool {
	var rpcErr *RpcError
	return errors.As(err, &rpcErr) && rpcErr.Code == 403
}

// IsMethodNotFound reports whether the server does not know the procedure, e.g. one
// that a plugin adds (getTaskGroupAssign) when the plugin is not installed.
func IsMethodNotFound(err error) bool {
	var rpcErr *RpcError
	return errors.As(err, &rpcErr) && rpcErr.Code == -32601
}

func WebClientRpcCall[REQ any, RESP any](kbCfg config.KanboardConfig, req REQ, resp *RESP) error {
	// Connect to server
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// https://docs.kanboard.org/v1/api/authentication/
	client := &http.Client{
		// Large requests (createTaskFile) get an extra second per 256 KiB.
		Timeout: time.Second*10 + time.Duration(len(data)/(256*1024))*time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			slog.Warn("Redirected",
				"from", via[len(via)-1].URL.String(), "to", req.URL.String(), "method", req.Method)
			return nil // Allows seeing if it drops the body
		},
	}

	httpReq, err := http.NewRequest("POST", kbCfg.ApiUrl, bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.SetBasicAuth(kbCfg.Username, kbCfg.Password)
	httpReq.Header.Set("Content-Type", "application/json")
	response, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if !json.Valid(bodyBytes) {
		answer, notice := splitPhpNotice(bodyBytes)
		if answer == nil {
			// e.g. a PHP fatal error page, or a proxy's error page
			return fmt.Errorf("Kanboard answered HTTP %v without JSON: %s", response.StatusCode, textSnippet(bodyBytes, 300))
		}
		// The call did run: failing here would make a retried write happen twice.
		slog.Warn("Kanboard printed a message before its answer", "method", methodOf(data), "message", textSnippet(notice, 300))
		bodyBytes = answer
	}

	var rpcErr KBResponseError
	err = json.Unmarshal(bodyBytes, &rpcErr)
	if err == nil {
		if rpcErr.JsonRpc != "2.0" {
			return fmt.Errorf(
				"json rpc protocol error, got jsonrpc=%v instead of 2.0", rpcErr.JsonRpc)
		}
		if rpcErr.Error.Message != "" {
			return &RpcError{
				HttpStatus: response.StatusCode,
				Code:       rpcErr.Error.Code,
				Message:    rpcErr.Error.Message,
				Data:       rpcErr.Error.Data,
			}
		}
	}
	err = json.Unmarshal(bodyBytes, &resp)
	if err != nil {
		return err
	}
	return nil
}

var (
	reHtmlTag = regexp.MustCompile(`<[^>]*>`)
	reSpaces  = regexp.MustCompile(`\s+`)
)

// textSnippet returns the start of an HTML or text body as one line of plain text.
func textSnippet(body []byte, n int) string {
	s := html.UnescapeString(reHtmlTag.ReplaceAllString(string(body), " "))
	s = strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
	if r := []rune(s); len(r) > n {
		s = string(r[:n]) + "..."
	}
	return s
}

// splitPhpNotice splits an answer that PHP prefixed with a notice or warning (with
// display_errors on, e.g. a plugin's deprecation warning) into the JSON-RPC answer and
// the notice. answer is nil if the body does not end with a JSON-RPC object.
func splitPhpNotice(body []byte) (answer, notice []byte) {
	i := bytes.Index(body, []byte(`{"jsonrpc"`))
	if i <= 0 || !json.Valid(body[i:]) {
		return nil, nil
	}
	return body[i:], body[:i]
}

// methodOf returns the method name of an encoded request, for logging.
func methodOf(request []byte) string {
	var r struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(request, &r)
	return r.Method
}
