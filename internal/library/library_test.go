package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestImportPersistenceRangeAndOffline(t *testing.T) {
	data := t.TempDir()
	root := filepath.Join(t.TempDir(), "media")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("0123456789video")
	path := filepath.Join(root, "sample.mp4")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := store.AddPool("影片庫", root)
	if err != nil {
		t.Fatal(err)
	}
	details := Metadata{Description: "測試內容\n第二行", Category: "教育", Adult: true, License: "保留所有權利"}
	details, _ = details.normalize()
	video, err := store.Import(context.Background(), pool.ID, "sample.mp4", "測試影片", details)
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(content)
	if video.FileHash != hex.EncodeToString(expected[:]) {
		t.Fatal("wrong hash")
	}
	if _, err := store.Import(context.Background(), pool.ID, "sample.mp4", ""); err == nil {
		t.Fatal("duplicate import accepted")
	}
	nodeID := store.nodeID
	store.Close()
	store, err = Open(data)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if nodeID != store.nodeID {
		t.Fatal("node identity changed on restart")
	}
	saved, err := store.video(video.ID)
	if err != nil || !reflect.DeepEqual(saved.Metadata, details) {
		t.Fatalf("metadata not persisted: %+v %v", saved.Metadata, err)
	}
	mux := http.NewServeMux()
	store.Register(mux)
	request := httptest.NewRequest("GET", "/api/v1/videos/"+video.ID+"/content", nil)
	request.Header.Set("Range", "bytes=2-5")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 206 || response.Body.String() != "2345" || response.Header().Get("Content-Range") != "bytes 2-5/15" {
		t.Fatalf("bad range: %d %s %v", response.Code, response.Body, response.Header())
	}
	request = httptest.NewRequest("HEAD", "/api/v1/videos/"+video.ID+"/content", nil)
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 200 || response.Body.Len() != 0 {
		t.Fatal("HEAD failed")
	}
	// 模擬來源離線；索引仍應存在並標示不可用。
	moved := root + "-offline"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(moved, root)
	videos, err := store.Videos()
	if err != nil || len(videos) != 1 || videos[0].Available {
		t.Fatalf("offline video lost: %+v %v", videos, err)
	}
	pools, err := store.Pools()
	if err != nil || pools[0].Online {
		t.Fatal("offline pool not detected")
	}
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/videos/"+video.ID+"/content", nil))
	if response.Code != 503 {
		t.Fatalf("offline content status %d", response.Code)
	}
}

func TestMigrateV1PreservesExistingVideo(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "old.mp4"), []byte("old video"), 0600); err != nil {
		t.Fatal(err)
	}
	pool, err := store.AddPool("existing", root)
	if err != nil {
		t.Fatal(err)
	}
	video, err := store.Import(context.Background(), pool.ID, "old.mp4", "old title")
	if err != nil {
		t.Fatal(err)
	}
	nodeID := store.nodeID
	// 還原 v1 欄位形狀，以既有資料驗證實際升級而非只測空資料庫。
	_, err = store.db.Exec(`ALTER TABLE videos DROP COLUMN description;
ALTER TABLE videos DROP COLUMN category;
ALTER TABLE videos DROP COLUMN adult;
ALTER TABLE videos DROP COLUMN license;
DROP TABLE video_tags;
DROP TABLE video_attribution;
DROP TABLE import_jobs;
DROP TABLE video_media;
DELETE FROM schema_version WHERE version>=2;`)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved, err := store.video(video.ID)
	defaults, _ := (Metadata{}).normalize()
	if err != nil || saved.Title != "old title" || saved.FileHash != video.FileHash || !reflect.DeepEqual(saved.Metadata, defaults) || store.nodeID != nodeID {
		t.Fatalf("migration lost existing data: %+v %v", saved, err)
	}
	if _, err := store.Import(context.Background(), pool.ID, "old.mp4", "", Metadata{Description: strings.Repeat("字", 10001)}); err == nil {
		t.Fatal("oversized description accepted")
	}
}

func TestImportRejectsEscapeAndCancelledRequest(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	parent := t.TempDir()
	root := filepath.Join(parent, "pool")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(parent, "outside.mp4"), []byte("outside"), 0600)
	os.WriteFile(filepath.Join(root, "inside.mp4"), []byte("inside"), 0600)
	pool, err := store.AddPool("pool", root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(context.Background(), pool.ID, "../outside.mp4", ""); err == nil {
		t.Fatal("path escape accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Import(ctx, pool.ID, "inside.mp4", ""); err == nil {
		t.Fatal("cancelled request committed")
	}
	videos, err := store.Videos()
	if err != nil || len(videos) != 0 {
		t.Fatal("invalid request left index")
	}
}

func TestManagementOriginAndChangedSource(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mux := http.NewServeMux()
	store.Register(mux)
	body, _ := json.Marshal(map[string]string{"name": "pool", "path": t.TempDir()})
	request := httptest.NewRequest("POST", "http://node/api/v1/pools", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://attacker.example")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("cross origin mutation allowed")
	}
	root := t.TempDir()
	path := filepath.Join(root, "file.mp4")
	os.WriteFile(path, []byte("original"), 0600)
	pool, err := store.AddPool("pool", root)
	if err != nil {
		t.Fatal(err)
	}
	video, err := store.Import(context.Background(), pool.ID, "file.mp4", "")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("modified content"), 0600)
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/videos/"+video.ID+"/content", nil))
	if response.Code != 503 {
		t.Fatal("modified content served with stale hash")
	}
}
