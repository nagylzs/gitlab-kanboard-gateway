package kanboard

import (
	"fmt"
	"testing"
)

func TestDecodeColorList(t *testing.T) {
	got, err := decodeColorList([]byte(` {"yellow":"Sárga","white_on_black":"White on Black","blue":"Kék"} `))
	if err != nil {
		t.Fatal(err)
	}
	// The server's order is kept.
	if fmt.Sprint(got) != "[{yellow Sárga} {white_on_black White on Black} {blue Kék}]" {
		t.Errorf("got %v", got)
	}
	for _, empty := range []string{"[]", "null", ""} {
		if got, err := decodeColorList([]byte(empty)); err != nil || len(got) != 0 {
			t.Errorf("%q: got %v, %v; want no colors", empty, got, err)
		}
	}
	for _, bad := range []string{`["yellow"]`, `{"yellow":1}`, `{"yellow":"Sárga"`} {
		if got, err := decodeColorList([]byte(bad)); err == nil {
			t.Errorf("%q: got %v, want an error", bad, got)
		}
	}
}
