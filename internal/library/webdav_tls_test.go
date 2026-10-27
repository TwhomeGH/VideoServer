package library

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebDAVHTTPClientTLSVerification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	store, _, _ := scannerFixture(t)
	verified := store.webDAVHTTPClient(Pool{RemoteURL: server.URL}, 2*time.Second)
	if response, err := verified.Get(server.URL); err == nil {
		response.Body.Close()
		t.Fatal("self-signed certificate was accepted by default")
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	custom := store.webDAVHTTPClient(Pool{RemoteURL: server.URL, TLSCA: string(caPEM)}, 2*time.Second)
	customResponse, err := custom.Get(server.URL)
	if err != nil {
		t.Fatalf("custom CA failed: %v", err)
	}
	customResponse.Body.Close()
	insecure := store.webDAVHTTPClient(Pool{RemoteURL: server.URL, TLSSkipVerify: true}, 2*time.Second)
	response, err := insecure.Get(server.URL)
	if err != nil {
		t.Fatalf("explicit TLS skip failed: %v", err)
	}
	defer response.Body.Close()
}
