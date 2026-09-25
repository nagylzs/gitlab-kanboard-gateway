package taskdump

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kanboard stores pasted screenshots as task attachments and inserts a relative
// <img> tag into the comment, e.g.
//
//	<img src="?controller=FileViewerController&action=image&task_id=13670&file_id=14789" class="enlargable" />
//
// Such URLs are useless outside the Kanboard web UI (relative, session-authenticated),
// so they are rewritten to a markdown image whose alt text is the attachment name and
// whose target is the downloaded local file (if any) or the absolute Kanboard URL.
// Any other relative FileViewerController URL in the text is made absolute.

var (
	reFileViewerImg = regexp.MustCompile(
		`(?is)<img\b[^>]*\bsrc="(\?controller=FileViewerController[^"]*?\bfile_id=(\d+)[^"]*)"[^>]*>`)
	// A relative URL starts the text or follows a non-"/" character; this keeps the
	// pass from re-matching URLs that were already made absolute.
	reFileViewerUrl = regexp.MustCompile(
		`(^|[^/])(\?controller=FileViewerController&(?:amp;)?[^\s"'<>)]*?\bfile_id=(\d+)[^\s"'<>)]*)`)
)

// rewriteEmbeddedFiles returns the rewritten comment text and the sorted, de-duplicated
// list of attachment ids referenced by it. baseUrl is the Kanboard application root
// (ending with "/"); attachments maps file id -> attachment already collected for the task.
func rewriteEmbeddedFiles(content string, baseUrl string, attachments map[int]*Attachment) (string, []int) {
	ids := make(map[int]struct{})
	note := func(id int) {
		ids[id] = struct{}{}
	}
	target := func(rel string, id int) string {
		if att, ok := attachments[id]; ok && att.LocalPath != "" {
			return att.LocalPath
		}
		return baseUrl + strings.ReplaceAll(rel, "&amp;", "&")
	}

	out := reFileViewerImg.ReplaceAllStringFunc(content, func(tag string) string {
		m := reFileViewerImg.FindStringSubmatch(tag)
		id, err := strconv.Atoi(m[2])
		if err != nil {
			return tag
		}
		note(id)
		alt := fmt.Sprintf("attachment %v", id)
		if att, ok := attachments[id]; ok {
			alt = att.Name
		}
		return fmt.Sprintf("![%s](%s)", alt, target(m[1], id))
	})

	out = reFileViewerUrl.ReplaceAllStringFunc(out, func(match string) string {
		m := reFileViewerUrl.FindStringSubmatch(match)
		prefix, rel := m[1], m[2]
		id, err := strconv.Atoi(m[3])
		if err != nil {
			return match
		}
		note(id)
		return prefix + target(rel, id)
	})

	if len(ids) == 0 {
		return out, nil
	}
	list := make([]int, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	sort.Ints(list)
	return out, list
}

// appBaseUrl derives the Kanboard application root from the jsonrpc endpoint URL:
// "https://kb.example.com/jsonrpc.php" -> "https://kb.example.com/".
func appBaseUrl(apiUrl string) string {
	if i := strings.LastIndex(apiUrl, "/"); i >= 0 {
		return apiUrl[:i+1]
	}
	return apiUrl
}
