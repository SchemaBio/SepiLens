package ssrf_test

import (
	"errors"
	"testing"

	"github.com/pzweuj/sepilens/internal/ssrf"
)

func TestSSRF_ValidateEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		allowPrivate bool
		wantErr      error
	}{
		{
			name:         "valid public https",
			url:          "https://api.openai.com/v1",
			allowPrivate: false,
			wantErr:      nil,
		},
		{
			name:         "reject http by default",
			url:          "http://api.openai.com/v1",
			allowPrivate: false,
			wantErr:      ssrf.ErrUnsupportedProto,
		},
		{
			name:         "reject loopback 127.0.0.1",
			url:          "https://127.0.0.1:8080/v1",
			allowPrivate: false,
			wantErr:      ssrf.ErrBlockedEndpoint,
		},
		{
			name:         "reject aws metadata 169.254.169.254",
			url:          "https://169.254.169.254/latest/meta-data",
			allowPrivate: false,
			wantErr:      ssrf.ErrBlockedEndpoint,
		},
		{
			name:         "reject private 10.0.0.1",
			url:          "https://10.0.0.1/v1",
			allowPrivate: false,
			wantErr:      ssrf.ErrBlockedEndpoint,
		},
		{
			name:         "reject private 192.168.1.1",
			url:          "https://192.168.1.1/v1",
			allowPrivate: false,
			wantErr:      ssrf.ErrBlockedEndpoint,
		},
		{
			name:         "reject localhost",
			url:          "https://localhost:8080/v1",
			allowPrivate: false,
			wantErr:      ssrf.ErrBlockedEndpoint,
		},
		{
			name:         "reject gcp metadata domain",
			url:          "https://metadata.google.internal/computeMetadata/v1",
			allowPrivate: false,
			wantErr:      ssrf.ErrBlockedEndpoint,
		},
		{
			name:         "allow private http when flag is enabled",
			url:          "http://127.0.0.1:8080/v1",
			allowPrivate: true,
			wantErr:      nil,
		},
		{
			name:         "invalid empty url",
			url:          "",
			allowPrivate: false,
			wantErr:      ssrf.ErrInvalidURL,
		},
		{
			name:         "invalid non-url",
			url:          "://bad",
			allowPrivate: false,
			wantErr:      ssrf.ErrInvalidURL,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ssrf.ValidateEndpoint(tc.url, tc.allowPrivate)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.wantErr)
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error wrapping %v, got %v", tc.wantErr, err)
				}
			}
		})
	}
}
