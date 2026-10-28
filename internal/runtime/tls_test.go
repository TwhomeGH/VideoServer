package runtime

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTLSFilesMustBeConfiguredTogether(t *testing.T) {
	base := Config{Host: "127.0.0.1", PreferredPort: 8080, DataDirectory: "data", FFprobePath: "ffprobe", ProbeTimeoutSeconds: 60, ScanIntervalSeconds: 30, ScanWarmSeconds: 300, ScanWarmSlowSeconds: 900, ScanColdSeconds: 1800, FileStableSeconds: 10, ScanIdleMaxSeconds: 21600, BackfillIntervalSeconds: 60}
	certOnly := base
	certOnly.TLSCertFile = "cert.pem"
	if err := certOnly.Validate(); err == nil {
		t.Fatal("certificate without key passed validation")
	}
	keyOnly := base
	keyOnly.TLSKeyFile = "key.pem"
	if err := keyOnly.Validate(); err == nil {
		t.Fatal("key without certificate passed validation")
	}
	both := base
	both.TLSCertFile = "cert.pem"
	both.TLSKeyFile = "key.pem"
	if err := both.Validate(); err != nil {
		t.Fatalf("valid TLS pair rejected: %v", err)
	}
}

func TestTLSFilesCanBeLoaded(t *testing.T) {
	directory := t.TempDir()
	certPath, keyPath := writeTestCertificate(t, directory)
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		t.Fatalf("test certificate did not load: %v", err)
	}
}

func TestHSTSHandlerOnlyOnTLS(t *testing.T) {
	handler := hstsHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	plain := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	plainResponse := httptest.NewRecorder()
	handler.ServeHTTP(plainResponse, plain)
	if plainResponse.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS was set on plain HTTP")
	}
	secure := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	secure.TLS = &tls.ConnectionState{}
	secureResponse := httptest.NewRecorder()
	handler.ServeHTTP(secureResponse, secure)
	if secureResponse.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("HSTS missing on TLS")
	}
}

func writeTestCertificate(t *testing.T, directory string) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(directory, "cert.pem")
	keyPath := filepath.Join(directory, "key.pem")
	certFile, err := os.Create(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	certFile.Close()
	keyFile, err := os.Create(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}); err != nil {
		t.Fatal(err)
	}
	keyFile.Close()
	return certPath, keyPath
}
