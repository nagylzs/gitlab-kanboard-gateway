package kanboard

import "testing"

func TestDecodeProjectUsers(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want map[int]string
	}{
		{`{"9":"Agent","4":"Ann","":null}`, map[int]string{9: "Agent", 4: "Ann"}},
		{`{"7":null}`, map[int]string{}},
		{`[]`, map[int]string{}},
		{`null`, map[int]string{}},
	} {
		got, err := decodeProjectUsers([]byte(tc.in))
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.in, got, tc.want)
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Fatalf("%s: got %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}
