package library

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetPoolTLSCanRotate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	store, _, _ := scannerFixture(t)
	pool, err := store.AddRemotePool("tls", server.URL+"/dav", "u", "p", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	updated, err := store.SetPoolTLS(pool.ID, string(caPEM), false)
	if err != nil || !updated.TLSCASet || updated.TLSSkipVerify {
		t.Fatalf("TLS rotation failed: %+v %v", updated, err)
	}
	if _, err := store.SetPoolTLS(pool.ID, "bad", false); err == nil {
		t.Fatal("invalid CA accepted")
	}
	if _, err := store.SetPoolTLS(pool.ID, string(caPEM), true); err == nil {
		t.Fatal("CA and skipVerify accepted together")
	}
	cleared, err := store.SetPoolTLS(pool.ID, "", true)
	if err != nil || cleared.TLSCASet || !cleared.TLSSkipVerify {
		t.Fatalf("TLS clear failed: %+v %v", cleared, err)
	}
}

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
