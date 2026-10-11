package ssrf

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

var (
	ErrInvalidURL       = errors.New("invalid endpoint URL")
	ErrUnsupportedProto = errors.New("endpoint protocol must be https")
	ErrBlockedEndpoint  = errors.New("endpoint resolves to a restricted network address or private host")
)

var privateCIDRs []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",    // IPv4 loopback
		"::1/128",        // IPv6 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // IPv4 Link-local / Cloud metadata (AWS, GCP, Azure)
		"fe80::/10",      // IPv6 Link-local
		"100.64.0.0/10",  // CGNAT
		"0.0.0.0/8",      // Current network / unspecified
		"::/128",         // IPv6 unspecified
		"192.0.0.0/24", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24",
		"198.18.0.0/15", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001:db8::/32", "2001:2::/48", "2001:10::/28", "2002::/16",
	}
	for _, c := range cidrs {
		_, ipnet, err := net.ParseCIDR(c)
		if err == nil {
			privateCIDRs = append(privateCIDRs, ipnet)
		}
	}
}

// ValidateEndpoint checks a user-provided LLM endpoint URL.
// When allowPrivate is false, only HTTPS to public addresses is permitted.
func ValidateEndpoint(rawURL string, allowPrivate bool) (*url.URL, error) {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return nil, fmt.Errorf("%w: empty URL", ErrInvalidURL)
	}

	u, err := url.ParseRequestURI(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if allowPrivate {
		if scheme != "https" && scheme != "http" {
			return nil, fmt.Errorf("%w: scheme %s is not permitted", ErrUnsupportedProto, scheme)
		}
	} else {
		if scheme != "https" {
			return nil, fmt.Errorf("%w: got %s, must be https", ErrUnsupportedProto, scheme)
		}
	}

	hostname := u.Hostname()
	if hostname == "" {
		return nil, fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	if u.User != nil || u.Fragment != "" {
		return nil, ErrInvalidURL
	}

	lowerHost := strings.ToLower(hostname)
	if !allowPrivate {
		if lowerHost == "localhost" ||
			strings.HasSuffix(lowerHost, ".local") ||
			strings.HasSuffix(lowerHost, ".internal") ||
			lowerHost == "metadata.google.internal" {
			return nil, fmt.Errorf("%w: restricted host %s", ErrBlockedEndpoint, hostname)
		}

		// Check if literal IP
		if ip := net.ParseIP(hostname); ip != nil {
			if isRestrictedIP(ip) {
				return nil, fmt.Errorf("%w: restricted IP %s", ErrBlockedEndpoint, hostname)
			}
		}
	}

	return u, nil
}

func isRestrictedIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	for _, block := range privateCIDRs {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
