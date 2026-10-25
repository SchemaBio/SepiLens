package ssrf

import (
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestNCBIFakeIPExceptionIsNarrow(t *testing.T) {
	for _, test := range []struct {
		host, port, ip string
		want           bool
	}{
		{"eutils.ncbi.nlm.nih.gov", "443", "198.18.0.151", true},
		{"eutils.ncbi.nlm.nih.gov", "443", "198.19.255.255", true},
		{"eutils.ncbi.nlm.nih.gov", "80", "198.18.0.151", false},
		{"evil.example", "443", "198.18.0.151", false},
		{"eutils.ncbi.nlm.nih.gov", "443", "127.0.0.1", false},
		{"eutils.ncbi.nlm.nih.gov", "443", "10.0.0.1", false},
		{"eutils.ncbi.nlm.nih.gov", "443", "169.254.169.254", false},
		{"eutils.ncbi.nlm.nih.gov", "443", "192.168.1.1", false},
	} {
		if got := permitsFakeIP(test.host, test.port, "eutils.ncbi.nlm.nih.gov", net.ParseIP(test.ip)); got != test.want {
			t.Fatal(test, got)
		}
	}
	for _, raw := range []string{"https://evil.example/entrez/eutils/esearch.fcgi", "http://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi", "https://eutils.ncbi.nlm.nih.gov:8443/entrez/eutils/esearch.fcgi", "https://eutils.ncbi.nlm.nih.gov/other", "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/../../other"} {
		request, _ := http.NewRequest("GET", raw, nil)
		if _, err := NewNCBIRetrievalClient(true).Do(request); !errors.Is(err, ErrBlockedEndpoint) {
			t.Fatal("NCBI client accepted unrelated endpoint", raw, err)
		}
	}
	resolver := &fixedResolver{ips: []net.IPAddr{{IP: net.ParseIP("198.18.0.151")}, {IP: net.ParseIP("127.0.0.1")}}}
	request, _ := http.NewRequest("GET", "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi", nil)
	if _, err := newHTTPClient(false, resolver, "eutils.ncbi.nlm.nih.gov").Do(request); !errors.Is(err, ErrBlockedEndpoint) {
		t.Fatal("mixed private DNS accepted", err)
	}
	resolver.ips = resolver.ips[:1]
	if _, err := NewHTTPClient(false, resolver).Do(request); !errors.Is(err, ErrBlockedEndpoint) {
		t.Fatal("default DNS guard weakened", err)
	}
}
