package bosk_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/prdoyle/terraform-provider-bosk/internal/bosk"
)

func TestConfigValidate(t *testing.T) {
	endpoint, err := url.Parse("https://example.com/bosk/state")
	if err != nil {
		t.Fatalf("parsing endpoint: %v", err)
	}

	tests := []struct {
		name string
		cfg  bosk.Config
		want string
	}{
		{
			name: "missing endpoint",
			cfg:  bosk.Config{},
			want: "endpoint is required",
		},
		{
			name: "bad scheme",
			cfg:  bosk.Config{Endpoint: &url.URL{Scheme: "ftp", Host: "example.com"}},
			want: "scheme",
		},
		{
			name: "missing host",
			cfg:  bosk.Config{Endpoint: &url.URL{Scheme: "https"}},
			want: "host",
		},
		{
			name: "query",
			cfg:  bosk.Config{Endpoint: &url.URL{Scheme: "https", Host: "example.com", RawQuery: "x=1"}},
			want: "query",
		},
		{
			name: "token and basic",
			cfg: bosk.Config{
				Endpoint: endpoint,
				Token:    "token",
				Username: "user",
				Password: "pass",
			},
			want: "mutually exclusive",
		},
		{
			name: "username without password",
			cfg: bosk.Config{
				Endpoint: endpoint,
				Username: "user",
			},
			want: "username and password",
		},
		{
			name: "reserved header",
			cfg: bosk.Config{
				Endpoint: endpoint,
				Headers:  map[string]string{"Authorization": "custom"},
			},
			want: "managed by the provider",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.cfg.Validate()
			if err == nil {
				t.Fatalf("Validate returned nil; want an error containing %q", test.want)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("Validate error = %q; want it to contain %q", err, test.want)
			}
		})
	}
}

func TestNewClientDefaults(t *testing.T) {
	endpoint, _ := url.Parse("https://example.com/bosk/state")
	client, err := bosk.NewClient(bosk.Config{Endpoint: endpoint})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient returned a nil client")
	}
}

func TestNewClientRejectsInvalidCACert(t *testing.T) {
	endpoint, _ := url.Parse("https://example.com/bosk/state")
	_, err := bosk.NewClient(bosk.Config{
		Endpoint: endpoint,
		CACert:   "not a certificate",
	})
	if err == nil {
		t.Fatal("NewClient returned nil error for an invalid CA certificate")
	}
	if !strings.Contains(err.Error(), "ca_cert") {
		t.Errorf("NewClient error = %q; want it to mention ca_cert", err)
	}
}
