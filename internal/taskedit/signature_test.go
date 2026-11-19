package taskedit

import (
	"strings"
	"testing"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

func TestSplitFooter(t *testing.T) {
	cases := []struct {
		text, body, created, modified string
		ok                            bool
	}{
		{"text\n\n---\n*created by Claude*", "text", "Claude", "", true},
		{"text\n\n---\n*created by Claude, modified by Bot*\n", "text", "Claude", "Bot", true},
		{"text\n\n---\n*modified by Bot*", "text", "", "Bot", true},
		{"---\n*created by Claude*", "", "Claude", "", true},
		{"a\n\n---\n\n*created by A \\*B\\**", "a", `A \*B\*`, "", true},
		{"a\r\n\r\n---\r\n*created by A*\r\n", "a", "A", "", true},
		{"text", "text", "", "", false},
		{"text\n\n---\n*something else*", "", "", "", false},
		{"text\n\n---\n*created by A*\n\nmore text", "", "", "", false},
		{"*created by A*", "", "", "", false},
	}
	for _, c := range cases {
		body, f, ok := splitFooter(c.text)
		if ok != c.ok || (ok && (body != c.body || f.created != c.created || f.modified != c.modified)) {
			t.Errorf("splitFooter(%q) = %q, %+v, %v; want %q, {%q %q}, %v",
				c.text, body, f, ok, c.body, c.created, c.modified, c.ok)
		}
	}
}

func TestSign(t *testing.T) {
	e := NewEditor(config.KanboardConfig{}, Options{Signature: "Claude"})
	if got := e.signCreated("Text\n\n"); got != "Text\n\n---\n*created by Claude*" {
		t.Errorf("signCreated = %q", got)
	}
	// A footer that the agent wrote itself is replaced, not kept below the new one.
	if got := e.signCreated("Text\n\n---\n*created by Someone*"); got != "Text\n\n---\n*created by Claude*" {
		t.Errorf("signCreated with footer = %q", got)
	}
	old := e.signCreated("v1")
	v2 := e.signModified(old, "v2")
	if v2 != "v2\n\n---\n*created by Claude, modified by Claude*" {
		t.Errorf("signModified = %q", v2)
	}
	// Editing again, with the old footer copied into the new text, keeps one footer.
	if v3 := e.signModified(v2, strings.Replace(v2, "v2", "v3", 1)); v3 != strings.Replace(v2, "v2", "v3", 1) {
		t.Errorf("signModified twice = %q", v3)
	}
	// Another signature keeps the creator.
	other := NewEditor(config.KanboardConfig{}, Options{Signature: "Bot_1"})
	if got := other.signModified(old, "v2"); got != "v2\n\n---\n*created by Claude, modified by Bot\\_1*" {
		t.Errorf("signModified by another signature = %q", got)
	}
	// No signature, no footer.
	plain := NewEditor(config.KanboardConfig{}, Options{})
	if got := plain.signCreated("Text"); got != "Text" {
		t.Errorf("signCreated without signature = %q", got)
	}
}

func TestEscapedSignatureRoundTrip(t *testing.T) {
	for _, sig := range []string{"Claude *Code*", "a_b", `back\slash`, "[x](y)", "<b>"} {
		e := NewEditor(config.KanboardConfig{}, Options{Signature: sig})
		_, f, ok := splitFooter(e.signCreated("text"))
		if !ok || f.created != mdEscape(sig) {
			t.Errorf("%q: footer %+v, ok=%v", sig, f, ok)
		}
	}
}

func TestValidateSignature(t *testing.T) {
	for _, ok := range []string{"", "Claude", "Claude Code (on ekke)", "Клод"} {
		if err := ValidateSignature(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"a\nb", "tab\there", " Claude", strings.Repeat("x", 101)} {
		if err := ValidateSignature(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestUserIdWithAppToken(t *testing.T) {
	e := NewEditor(config.KanboardConfig{Username: config.AppTokenUsername}, Options{})
	if _, err := e.UserId(); err == nil || !strings.Contains(err.Error(), "Kanboard.UserId") {
		t.Errorf("want an error about Kanboard.UserId, got %v", err)
	}
	e = NewEditor(config.KanboardConfig{Username: config.AppTokenUsername, UserId: 5}, Options{})
	if id, err := e.UserId(); err != nil || id != 5 {
		t.Errorf("UserId = %v, %v", id, err)
	}
}

func TestPick(t *testing.T) {
	items := []named{{id: 1, name: "Backlog"}, {id: 2, name: "Work in progress"}, {id: 3, name: "Work done"}, {id: 4, name: "Ready", alt: "rdy"}}
	for ref, want := range map[string]int{"1": 1, "backlog": 1, "progress": 2, "WORK DONE": 3, "rdy": 4} {
		if got, err := pick("column", ref, items); err != nil || got.id != want {
			t.Errorf("pick(%q) = %v, %v; want %v", ref, got.id, err, want)
		}
	}
	for ref, want := range map[string]string{"work": "more than one", "9": "no column with id 9", "zzz": "choose one of"} {
		if _, err := pick("column", ref, items); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("pick(%q): %v, want an error containing %q", ref, err, want)
		}
	}
}
