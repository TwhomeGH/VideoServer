package library

import (
	"crypto/tls"
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
		}
		transport.TLSClientConfig = config
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}
