package secure

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCipherBinding(t *testing.T) {
	c, e := New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if e != nil {
		t.Fatal(e)
	}
	s, e := c.Seal("patient", "tenant:one")
	if e != nil || strings.Contains(s, "patient") {
		t.Fatal(e)
	}
	p, e := c.Open(s, "tenant:one")
	if e != nil || p != "patient" {
		t.Fatal(e)
	}
	if _, e = c.Open(s, "tenant:two"); e == nil {
		t.Fatal("cross tenant decrypt")
	}
	if _, e = c.Open(s+"x", "tenant:one"); e == nil {
		t.Fatal("tampering accepted")
	}
}
