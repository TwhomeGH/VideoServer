package library

import (
	"context"
	"testing"
	"time"
)

func TestRemoteRenameMapRequiresUniqueIdentity(t *testing.T) {
	existing := []discoveredIdentity{
		{Relative: "old.mp4", ETag: `"same"`, Size: 10},
		{Relative: "dup-a.mp4", ETag: `"dup"`, Size: 20},
		{Relative: "dup-b.mp4", ETag: `"dup"`, Size: 20},
	}
	entries := []webDAVEntry{
		{Relative: "new.mp4", ETag: `"same"`, Size: 10},
		{Relative: "dup-new.mp4", ETag: `"dup"`, Size: 20},
	}
	renames := remoteRenameMap(existing, entries)
	if renames["old.mp4"] != "new.mp4" {
		t.Fatalf("unique rename not detected: %+v", renames)
	}
	if _, exists := renames["dup-a.mp4"]; exists {
		t.Fatalf("ambiguous rename should be ignored: %+v", renames)
	}
}

func TestUpsertDiscoveredUpdatesETag(t *testing.T) {
	store, _, pool := scannerFixture(t)
	at := time.Now().UTC()
	if _, err := store.upsertDiscovered(context.Background(), pool.ID, "file.mp4", 10, at, `"one"`, at); err != nil {
		t.Fatal(err)
	}
	changed, err := store.upsertDiscovered(context.Background(), pool.ID, "file.mp4", 10, at, `"two"`, at)
	if err != nil || !changed {
		t.Fatalf("etag change: changed=%v err=%v", changed, err)
	}
	item, err := store.discoveredItem(context.Background(), pool.ID, "file.mp4")
	if err != nil || item.ETag != `"two"` {
		t.Fatalf("etag not updated: %+v %v", item, err)
	}
}

func TestRenameRemotePathUpdatesDiscovery(t *testing.T) {
	store, _, pool := scannerFixture(t)
	at := time.Now().UTC()
	if _, err := store.upsertDiscovered(context.Background(), pool.ID, "old.mp4", 123, at, `"etag"`, at); err != nil {
		t.Fatal(err)
	}
	if err := store.renameRemotePath(context.Background(), pool.ID, "old.mp4", "new.mp4"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.discoveredItem(context.Background(), pool.ID, "old.mp4"); err == nil {
		t.Fatal("old discovered path still exists")
	}
	item, err := store.discoveredItem(context.Background(), pool.ID, "new.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if item.Size != 123 || item.ETag != `"etag"` {
		t.Fatalf("renamed discovery lost metadata: %+v", item)
	}
}
