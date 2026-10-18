package bosk_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prdoyle/terraform-provider-bosk/internal/bosk"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating CA key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generating CA serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "terraform-provider-bosk test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing CA certificate: %v", err)
	}
	return &testCA{
		cert: cert,
		key:  key,
		pem:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
}

func (ca *testCA) serverCertificate(t *testing.T, dnsNames []string, ipAddresses []net.IP) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating server key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generating server serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "terraform-provider-bosk test server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("creating server certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling server key: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("building server key pair: %v", err)
	}
	return cert
}

func startTLSServer(t *testing.T, cert tls.Certificate) *httptest.Server {
	t.Helper()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	server.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func newTLSClient(t *testing.T, endpoint string, caPEM []byte, insecure bool) *bosk.Client {
	t.Helper()

	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parsing endpoint: %v", err)
	}
	client, err := bosk.NewClient(bosk.Config{
		Endpoint:           parsed,
		CACert:             string(caPEM),
		InsecureSkipVerify: insecure,
		RequestTimeout:     2 * time.Second,
		Retry: bosk.RetryPolicy{
			MaxAttempts: 1,
			BaseDelay:   time.Millisecond,
			MaxDelay:    time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	return client
}

func TestClientTLSCustomCA(t *testing.T) {
	ca := newTestCA(t)
	server := startTLSServer(t, ca.serverCertificate(t, nil, []net.IP{net.ParseIP("127.0.0.1")}))
	client := newTLSClient(t, server.URL, ca.pem, false)

	response, err := client.Get(context.Background(), "/", bosk.GetOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want %d", response.StatusCode, http.StatusOK)
	}
}

func TestClientTLSRejectsUntrustedCA(t *testing.T) {
	ca := newTestCA(t)
	server := startTLSServer(t, ca.serverCertificate(t, nil, []net.IP{net.ParseIP("127.0.0.1")}))
	client := newTLSClient(t, server.URL, nil, false)

	if _, err := client.Get(context.Background(), "/", bosk.GetOptions{Fresh: true}); err == nil {
		t.Fatal("Get succeeded with an untrusted CA")
	}
}

func TestClientTLSRejectsWrongCA(t *testing.T) {
	ca := newTestCA(t)
	server := startTLSServer(t, ca.serverCertificate(t, nil, []net.IP{net.ParseIP("127.0.0.1")}))
	otherCA := newTestCA(t)
	client := newTLSClient(t, server.URL, otherCA.pem, false)

	if _, err := client.Get(context.Background(), "/", bosk.GetOptions{Fresh: true}); err == nil {
		t.Fatal("Get succeeded with the wrong CA")
	}
}

func TestClientTLSInsecureSkipVerify(t *testing.T) {
	ca := newTestCA(t)
	server := startTLSServer(t, ca.serverCertificate(t, nil, []net.IP{net.ParseIP("127.0.0.1")}))
	client := newTLSClient(t, server.URL, nil, true)

	if _, err := client.Get(context.Background(), "/", bosk.GetOptions{Fresh: true}); err != nil {
		t.Fatalf("Get with insecure_skip_verify returned error: %v", err)
	}
}

func TestClientTLSHostnameMismatch(t *testing.T) {
	ca := newTestCA(t)
	server := startTLSServer(t, ca.serverCertificate(t, []string{"example.com"}, nil))
	client := newTLSClient(t, server.URL, ca.pem, false)

	_, err := client.Get(context.Background(), "/", bosk.GetOptions{Fresh: true})
	if err == nil {
		t.Fatal("Get succeeded despite a hostname mismatch")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "certificate") {
		t.Errorf("error = %v; want a certificate validation error", err)
	}
}
