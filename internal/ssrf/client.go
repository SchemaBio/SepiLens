package ssrf

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// NewHTTPClient pins each connection to a validated DNS answer and disables proxy/redirect bypasses.
func NewHTTPClient(allowPrivate bool, resolver Resolver) *http.Client {
	return newHTTPClient(allowPrivate, resolver, "")
}

// NewNCBIRetrievalClient explicitly supports local Fake-IP DNS for this fixed
// official HTTPS endpoint only. TLS verification and redirect blocking remain
// enabled; RFC1918, loopback, link-local and other private ranges stay blocked.
func NewNCBIRetrievalClient(allowFakeIP bool) *http.Client {
	if !allowFakeIP {
		return NewHTTPClient(false, nil)
	}
	return newHTTPClient(false, nil, "eutils.ncbi.nlm.nih.gov")
}

func newHTTPClient(allowPrivate bool, resolver Resolver, fakeIPHost string) *http.Client {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ForceAttemptHTTP2: true, MaxIdleConns: 32, IdleConnTimeout: 60 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := resolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("endpoint DNS resolution failed")
		}
		for _, ip := range ips {
			if !allowPrivate && isRestrictedIP(ip.IP) && !permitsFakeIP(host, port, fakeIPHost, ip.IP) {
				return nil, ErrBlockedEndpoint
			}
		}
		var conn net.Conn
		for _, ip := range ips {
			conn, err = d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, errors.New("endpoint connection failed")
	}
	return &http.Client{Transport: &guardTransport{next: tr, allowPrivate: allowPrivate, fakeIPHost: fakeIPHost}, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("endpoint redirects are disabled") }}
}

type guardTransport struct {
	next         http.RoundTripper
	allowPrivate bool
	fakeIPHost   string
}

func (g *guardTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if g.fakeIPHost != "" && (!strings.EqualFold(r.URL.Hostname(), g.fakeIPHost) || r.URL.Scheme != "https" || (r.URL.Port() != "" && r.URL.Port() != "443") || !strings.HasPrefix(path.Clean(r.URL.Path), "/entrez/eutils/")) {
		return nil, ErrBlockedEndpoint
	}
	if _, err := ValidateEndpoint(r.URL.String(), g.allowPrivate); err != nil {
		return nil, err
	}
	return g.next.RoundTrip(r)
}

func benchmarkIP(ip net.IP) bool {
	v := ip.To4()
	return v != nil && v[0] == 198 && (v[1] == 18 || v[1] == 19)
}

func permitsFakeIP(host, port, trustedHost string, ip net.IP) bool {
	return trustedHost != "" && strings.EqualFold(host, trustedHost) && port == "443" && benchmarkIP(ip)
}
