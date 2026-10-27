package library

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"strings"
	"time"
)

// webDAVHTTPClient 建立共用的 WebDAV 用戶端；只有明確 opt-in 才跳過 TLS 驗證。
func (s *Store) webDAVHTTPClient(pool Pool, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.HasPrefix(strings.ToLower(pool.RemoteURL), "https://") {
		config := &tls.Config{MinVersion: tls.VersionTLS12}
		if pool.TLSSkipVerify {
			config.InsecureSkipVerify = true
		} else if pool.TLSCA != "" {
			roots, err := x509.SystemCertPool()
			if err != nil {
				roots = x509.NewCertPool()
			}
			if roots.AppendCertsFromPEM([]byte(pool.TLSCA)) {
				config.RootCAs = roots
			}
		}
		transport.TLSClientConfig = config
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}
