package bosk

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config describes how to reach a bosk maintenance endpoint.
// It contains no Terraform types so it can be reused by other clients.
type Config struct {
	Endpoint           *url.URL
	Token              string
	Username           string
	Password           string
	Headers            map[string]string
	RequestTimeout     time.Duration
	CACert             string
	InsecureSkipVerify bool
	UserAgent          string

	// Retry controls transient-failure retries. The zero value uses
	// DefaultRetryPolicy.
	Retry RetryPolicy
}

// RetryPolicy controls retries for transient HTTP failures.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 5,
		BaseDelay:   200 * time.Millisecond,
		MaxDelay:    5 * time.Second,
	}
}

func (p RetryPolicy) normalized() RetryPolicy {
	defaults := DefaultRetryPolicy()
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaults.MaxAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = defaults.BaseDelay
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = defaults.MaxDelay
	}
	if p.MaxDelay < p.BaseDelay {
		p.MaxDelay = p.BaseDelay
	}
	return p
}

// Validate checks the configuration before a client is constructed.
func (c Config) Validate() error {
	if c.Endpoint == nil {
		return fmt.Errorf("endpoint is required")
	}
	if c.Endpoint.Scheme != "http" && c.Endpoint.Scheme != "https" {
		return fmt.Errorf("endpoint scheme must be http or https, got %q", c.Endpoint.Scheme)
	}
	if c.Endpoint.Host == "" {
		return fmt.Errorf("endpoint must include a host")
	}
	if c.Endpoint.User != nil {
		return fmt.Errorf("endpoint must not include user information")
	}
	if c.Endpoint.RawQuery != "" || c.Endpoint.Fragment != "" {
		return fmt.Errorf("endpoint must not include a query or fragment")
	}
	if (c.Username == "") != (c.Password == "") {
		return fmt.Errorf("username and password must be supplied together")
	}
	if c.Token != "" && c.Username != "" {
		return fmt.Errorf("token and username/password are mutually exclusive")
	}
	if c.RequestTimeout < 0 {
		return fmt.Errorf("request timeout must not be negative")
	}
	for name := range c.Headers {
		if reservedHeader(name) {
			return fmt.Errorf("header %q is managed by the provider and cannot be overridden", name)
		}
	}
	return nil
}

func (c Config) httpClient() (*http.Client, error) {
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{}
	}
	transport := baseTransport.Clone()

	if c.CACert != "" || c.InsecureSkipVerify {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			// The user explicitly opts into skipping verification.
			InsecureSkipVerify: c.InsecureSkipVerify, //nolint:gosec
		}
		if c.CACert != "" {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM([]byte(c.CACert)) {
				return nil, fmt.Errorf("ca_cert does not contain a valid PEM certificate")
			}
			tlsConfig.RootCAs = pool
		}
		transport.TLSClientConfig = tlsConfig
	}

	return &http.Client{
		Transport: transport,
		// Each request gets its own context timeout. A client-wide timeout
		// would cut off long server-side flushes.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func reservedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "accept", "authorization", "cache-control", "content-type", "if-match", "if-none-match", "user-agent":
		return true
	default:
		return false
	}
}
