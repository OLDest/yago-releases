package music

import (
	"strings"
	"testing"
)

func TestAPIEnvelopePresenceAndErrors(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{{`{"result":{"title":"music"},"error":null}`, true}, {`{"result":null}`, false}, {`{}`, false}, {`{"error":{"name":"rejected"}}`, false}, {`{"result":{"title":"music"},"error":"rejected"}`, false}, {`{"result":`, false}} {
		var out struct{ Title string }
		e := decodeAPIResponse(strings.NewReader(tc.body), false, &out)
		if (e == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.body, e)
		}
		if tc.ok && out.Title != "music" {
			t.Fatal(out)
		}
	}
	if e := decodeAPIResponse(strings.NewReader(`{"result":null}`), false, nil); e != nil {
		t.Fatal("feedback null", e)
	}
}
