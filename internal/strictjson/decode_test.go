package strictjson

import "testing"

func TestAmbiguousAndUnknownJSONRejected(t *testing.T) {
	for _, raw := range []string{`{"a":1,"a":2}`, `{"a":1,"\u0061":2}`, `{"a":1} {}`, `{"unknown":1}`, `{"a":NaN}`} {
		var v struct {
			A int `json:"a"`
		}
		if Decode([]byte(raw), &v) == nil {
			t.Fatal(raw)
		}
	}
	var v struct {
		A int `json:"a"`
	}
	if Decode([]byte(`{"a":1}`), &v) != nil || v.A != 1 {
		t.Fatal("valid JSON rejected")
	}
}
