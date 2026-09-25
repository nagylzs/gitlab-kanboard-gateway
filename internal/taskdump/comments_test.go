package taskdump

import (
	"reflect"
	"testing"
)

func TestRewriteEmbeddedFiles(t *testing.T) {
	base := "https://kb.example.com/"
	atts := map[int]*Attachment{
		14789: {Id: 14789, Name: "shot.png", LocalPath: "/tmp/att/13670/14789-shot.png"},
		14790: {Id: 14790, Name: "other.png"}, // not downloaded
	}
	cases := []struct {
		name    string
		in      string
		wantOut string
		wantIds []int
	}{
		{
			name:    "img tag, downloaded",
			in:      `before <img src="?controller=FileViewerController&action=image&task_id=13670&file_id=14789" class="enlargable" /> after`,
			wantOut: `before ![shot.png](/tmp/att/13670/14789-shot.png) after`,
			wantIds: []int{14789},
		},
		{
			name:    "img tag, not downloaded -> absolute url, html entities decoded",
			in:      `<img src="?controller=FileViewerController&amp;action=image&amp;task_id=13670&amp;file_id=14790">`,
			wantOut: `![other.png](https://kb.example.com/?controller=FileViewerController&action=image&task_id=13670&file_id=14790)`,
			wantIds: []int{14790},
		},
		{
			name:    "unknown attachment id keeps generic alt text",
			in:      `<img src="?controller=FileViewerController&action=image&task_id=1&file_id=5"/>`,
			wantOut: `![attachment 5](https://kb.example.com/?controller=FileViewerController&action=image&task_id=1&file_id=5)`,
			wantIds: []int{5},
		},
		{
			name:    "bare relative url in markdown link",
			in:      `see [file](?controller=FileViewerController&action=download&task_id=13670&file_id=14790) here`,
			wantOut: `see [file](https://kb.example.com/?controller=FileViewerController&action=download&task_id=13670&file_id=14790) here`,
			wantIds: []int{14790},
		},
		{
			name:    "two images, ids sorted and de-duplicated",
			in:      `<img src="?controller=FileViewerController&action=image&task_id=1&file_id=14790"/> x <img src="?controller=FileViewerController&action=image&task_id=1&file_id=14789"/> y ?controller=FileViewerController&action=image&task_id=1&file_id=14790`,
			wantOut: `![other.png](https://kb.example.com/?controller=FileViewerController&action=image&task_id=1&file_id=14790) x ![shot.png](/tmp/att/13670/14789-shot.png) y https://kb.example.com/?controller=FileViewerController&action=image&task_id=1&file_id=14790`,
			wantIds: []int{14789, 14790},
		},
		{
			name:    "plain text untouched",
			in:      "nothing to see <b>here</b> ![x](https://elsewhere/img.png)",
			wantOut: "nothing to see <b>here</b> ![x](https://elsewhere/img.png)",
			wantIds: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, ids := rewriteEmbeddedFiles(c.in, base, atts)
			if out != c.wantOut {
				t.Errorf("content:\n got: %s\nwant: %s", out, c.wantOut)
			}
			if !reflect.DeepEqual(ids, c.wantIds) {
				t.Errorf("ids: got %v want %v", ids, c.wantIds)
			}
		})
	}
}

func TestAppBaseUrl(t *testing.T) {
	if got := appBaseUrl("https://kb.example.com/jsonrpc.php"); got != "https://kb.example.com/" {
		t.Errorf("got %q", got)
	}
	if got := appBaseUrl("https://kb.example.com/sub/dir/jsonrpc.php"); got != "https://kb.example.com/sub/dir/" {
		t.Errorf("got %q", got)
	}
}
