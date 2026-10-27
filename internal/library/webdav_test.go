package library

import "testing"

func TestAddRemotePoolPersistsCredentials(t *testing.T) {
	store, _, _ := scannerFixture(t)
	pool, err := store.AddRemotePool("OpenList", "https://example.test/dav", "guest", "secret", "Video5", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if pool.Provider != "webdav" || !pool.IndexOnly || pool.Filesystem != "WebDAV" {
		t.Fatalf("unexpected remote pool: %+v", pool)
	}
	username, password, err := store.WebDAVCredentials(pool.ID)
	if err != nil || username != "guest" || password != "secret" {
		t.Fatalf("credentials = %q/%q, %v", username, password, err)
	}
	pools, err := store.Pools()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range pools {
		if candidate.ID == pool.ID {
			found = true
			if candidate.RemoteURL != "https://example.test/dav" || candidate.RemoteRoot != "Video5" {
				t.Fatalf("remote metadata = %+v", candidate)
			}
		}
	}
	if !found {
		t.Fatal("remote pool was not listed")
	}
}
