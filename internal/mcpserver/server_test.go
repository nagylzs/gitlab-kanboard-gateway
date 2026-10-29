package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

const fakeProject = `{"id":7,"name":"Web","is_active":true,"url":{"board":"http://kb/board/7"}}`

// fakeKanboard answers the read-only procedures the tools use and records the
// called method names.
type fakeKanboard struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeKanboard) called(method string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.calls, method)
}

func (f *fakeKanboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	param := func(name string) int {
		n, _ := req.Params[name].(float64)
		return int(n)
	}
	f.mu.Lock()
	f.calls = append(f.calls, req.Method)
	f.mu.Unlock()

	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 24)...)
	files := map[int]string{
		5: `{"id":5,"name":"shot.png","is_image":true,"task_id":1,"size":32}`,
		6: `{"id":6,"name":"build.log","task_id":1,"size":11}`,
		7: `{"id":7,"name":"huge.png","is_image":true,"task_id":1,"size":5000}`,
		8: `{"id":8,"name":"logs.zip","task_id":1,"size":8}`,
	}
	contents := map[int][]byte{5: png, 6: []byte("hello build"), 8: []byte("PK\x03\x04\x00\x00\x00\x00")}

	result := "null"
	switch req.Method {
	case "getTask":
		if param("task_id") == 1 {
			result = `{"id":1,"title":"Login fails","description":"a <b> & c","project_id":7,"is_active":true,"url":"http://kb/task/1"}`
		}
	case "getAllComments":
		result = `[{"id":1,"comment":"first","username":"ann"},{"id":2,"comment":"second"},{"id":3,"comment":"third"}]`
	case "getProjectById":
		result = fakeProject
	case "getAllProjects":
		result = "[" + fakeProject + "]"
	case "searchTasks":
		result = `[{"id":1,"title":"Login fails","is_active":true,"project_id":7,"project_name":"Web"}]`
	case "getTaskFile":
		if s, ok := files[param("file_id")]; ok {
			result = s
		}
	case "downloadTaskFile":
		result = fmt.Sprintf("%q", base64.StdEncoding.EncodeToString(contents[param("file_id")]))
	default:
		if strings.HasPrefix(req.Method, "getAll") {
			result = "[]"
		}
	}
	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, result)
}

func connect(t *testing.T) (*mcp.ClientSession, *fakeKanboard) {
	t.Helper()
	fake := &fakeKanboard{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	cfg := config.KanboardConfig{ApiUrl: srv.URL + "/jsonrpc.php", Username: "jsonrpc", Password: "x"}

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := New(cfg, Options{Version: "test", MaxAttachmentBytes: 1000})
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, fake
}

// call returns the tool result; a tool error (IsError) is returned as the text.
func call(t *testing.T, session *mcp.ClientSession, tool string, args map[string]any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%v: %v", tool, err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("%v: empty result", tool)
	}
	text, _ := res.Content[0].(*mcp.TextContent)
	if text == nil {
		t.Fatalf("%v: first content is %T, not text", tool, res.Content[0])
	}
	return res, text.Text
}

func TestListTools(t *testing.T) {
	session, _ := connect(t)
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%v is not marked read-only", tool.Name)
		}
		if tool.Name == "get_task" {
			schema, _ := json.Marshal(tool.InputSchema)
			if !strings.Contains(string(schema), `"enum":["id","url","title"`) {
				t.Errorf("get_task fields have no enum: %s", schema)
			}
		}
	}
	slices.Sort(names)
	if want := []string{"get_attachment", "get_task", "search_tasks"}; !slices.Equal(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}
}

func TestGetTaskFieldsAndMaxComments(t *testing.T) {
	session, fake := connect(t)
	res, text := call(t, session, "get_task", map[string]any{
		"tasks": []string{"#KB1"}, "fields": []string{"comments", "title"}, "max_comments": 2,
	})
	if res.IsError {
		t.Fatal(text)
	}
	// Selected fields keep the document order, not the order of the request.
	if !strings.HasPrefix(text, `[{"id":1,"title":"Login fails","comments":[`) {
		t.Errorf("unexpected output: %s", text)
	}
	var docs []struct {
		Description     *string `json:"description"`
		Comments        []struct{ Id int }
		CommentsOmitted int `json:"comments_omitted"`
	}
	if err := json.Unmarshal([]byte(text), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Description != nil {
		t.Fatalf("unexpected docs: %s", text)
	}
	if ids := []int{docs[0].Comments[0].Id, docs[0].Comments[1].Id}; len(docs[0].Comments) != 2 || ids[0] != 2 || ids[1] != 3 {
		t.Errorf("want the newest 2 comments (2, 3), got %s", text)
	}
	if docs[0].CommentsOmitted != 1 {
		t.Errorf("comments_omitted = %v, want 1", docs[0].CommentsOmitted)
	}
	for _, skipped := range []string{"getProjectById", "getAllSubtasks", "getAllTaskLinks", "getTaskTags"} {
		if fake.called(skipped) {
			t.Errorf("%v was called although its field was not requested", skipped)
		}
	}
	if !fake.called("getAllTaskFiles") {
		t.Error("attachments must be loaded for comments (embedded images)")
	}
}

func TestGetTaskPartialFailure(t *testing.T) {
	session, _ := connect(t)
	res, text := call(t, session, "get_task", map[string]any{"tasks": []string{"1", "2"}})
	if res.IsError {
		t.Fatal(text)
	}
	if !strings.Contains(text, `"description":"a <b> & c"`) {
		t.Errorf("markdown should not be HTML-escaped: %s", text)
	}
	if !strings.HasSuffix(text, `{"id":2,"error":"task 2 not found"}]`) {
		t.Errorf("missing error entry for task 2: %s", text)
	}
}

func TestToolErrors(t *testing.T) {
	session, _ := connect(t)
	cases := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"get_task", map[string]any{"tasks": []string{"2"}}, "task 2 not found"},
		{"get_task", map[string]any{"tasks": []string{"1"}, "fields": []string{"bogus"}}, "fields"},
		{"get_task", map[string]any{"tasks": []string{}}, "tasks"},
		{"get_task", map[string]any{"tasks": []string{"abc"}}, "abc"},
		{"get_task", map[string]any{"tasks": []string{"1"}, "max_comments": -1}, "max_comments"},
		{"search_tasks", map[string]any{"query": ""}, "query"},
		{"get_attachment", map[string]any{"file_id": 7}, "more than the 1000 bytes"},
		{"get_attachment", map[string]any{"file_id": 8}, "cannot be shown"},
		{"get_attachment", map[string]any{"file_id": 9}, "not found"},
	}
	for _, c := range cases {
		res, text := call(t, session, c.tool, c.args)
		if !res.IsError || !strings.Contains(text, c.want) {
			t.Errorf("%v %v: IsError=%v text=%q, want an error containing %q", c.tool, c.args, res.IsError, text, c.want)
		}
	}
}

func TestSearchTasks(t *testing.T) {
	session, _ := connect(t)
	res, text := call(t, session, "search_tasks", map[string]any{"query": "login"})
	if res.IsError {
		t.Fatal(text)
	}
	var out struct {
		ProjectsSearched int `json:"projects_searched"`
		Total            int
		Tasks            []struct {
			Id  int
			Url string
		}
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out.ProjectsSearched != 1 || out.Total != 1 || !strings.HasSuffix(out.Tasks[0].Url, "/task/1") {
		t.Errorf("unexpected search result: %s", text)
	}
}

func TestGetAttachment(t *testing.T) {
	session, _ := connect(t)
	res, _ := call(t, session, "get_attachment", map[string]any{"file_id": 5})
	if image, ok := res.Content[1].(*mcp.ImageContent); res.IsError || !ok || image.MIMEType != "image/png" {
		t.Errorf("want a png image, got %#v", res.Content)
	}
	res, _ = call(t, session, "get_attachment", map[string]any{"file_id": 6})
	if text, ok := res.Content[1].(*mcp.TextContent); res.IsError || !ok || text.Text != "hello build" {
		t.Errorf("want the text file content, got %#v", res.Content)
	}
}
