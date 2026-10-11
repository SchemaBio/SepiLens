package ssrf

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fixedResolver struct {
	ips   []net.IPAddr
	calls int
}

func (r *fixedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	r.calls++
	return r.ips, nil
}
func TestMixedDNSAnswersRejectedBeforeDial(t *testing.T) {
	r := &fixedResolver{ips: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("127.0.0.1")}}}
	c := NewHTTPClient(false, r)
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	_, e := c.Do(req)
	if !errors.Is(e, ErrBlockedEndpoint) || r.calls != 1 {
		t.Fatal("mixed DNS bypass", e, r.calls)
	}
}
func TestRedirectCannotCarryCredentials(t *testing.T) {
	hit := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer origin.Close()
	c := NewHTTPClient(true, nil)
	req, _ := http.NewRequest("GET", origin.URL, nil)
	req.Header.Set("Authorization", "Bearer synthetic")
	res, e := c.Do(req)
	if res != nil {
		res.Body.Close()
	}
	if e == nil || hit {
		t.Fatal("redirect followed", e)
	}
}
