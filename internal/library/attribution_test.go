package library

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAttributionValidation(t *testing.T) {
	metadata, err := (Metadata{Tags: []string{" Go ", "go", "後端", "", "Go"}}).normalize()
	if err != nil || !reflect.DeepEqual(metadata.Tags, []string{"Go", "後端"}) || metadata.WorkType != "unknown" {
		t.Fatalf("bad normalized tags: %+v %v", metadata, err)
	}
	cases := []Metadata{
		{WorkType: "invalid"},
		{WorkType: "repost", SourcePending: true},
		{WorkType: "repost", Creators: []Creator{{Name: "作者"}}},
		{Creators: []Creator{{Name: "作者", URL: "javascript:alert(1)"}}},
		{SourceURL: "https://name:password@example.com/video"},
		{SourceURL: "file:///etc/passwd"},
		{Tags: []string{strings.Repeat("字", 41)}},
	}
	for i, metadata := range cases {
		if _, err := metadata.normalize(); err == nil {
			t.Fatalf("invalid metadata accepted, case %d", i)
		}
	}
	tooMany := Metadata{}
	for i := 0; i < 21; i++ {
		tooMany.Tags = append(tooMany.Tags, strings.Repeat("a", i+1))
	}
	if _, err := tooMany.normalize(); err == nil {
		t.Fatal("more than 20 tags accepted")
	}
	pending := Metadata{WorkType: "repost", Creators: []Creator{{Name: "作者"}}, SourcePending: true}
	if _, err := pending.normalize(); err != nil {
		t.Fatal(err)
	}
	pending.SourceURL = "https://example.com/video"
	pending, err = pending.normalize()
	if err != nil || pending.SourcePending {
		t.Fatal("source link should resolve pending flag")
	}
}

func TestMetadataAPIUpdateAndV2Migration(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "video.mp4"), []byte("video content"), 0600); err != nil {
		t.Fatal(err)
	}
	pool, err := store.AddPool("pool", root)
	if err != nil {
		t.Fatal(err)
	}
	initial := Metadata{Description: "既有簡介", Category: "教育", Adult: true, License: "保留所有權利"}
	video, err := store.Import(context.Background(), pool.ID, "video.mp4", "existing title", initial)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := store.nodeID
	_, err = store.db.Exec("DROP TABLE video_tags; DROP TABLE video_attribution; DELETE FROM schema_version WHERE version=3")
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	store.Register(mux)
	saved, err := store.video(video.ID)
	if err != nil || saved.Description != initial.Description || saved.WorkType != "unknown" || store.nodeID != nodeID {
		store.Close()
		t.Fatal("v2 migration lost metadata")
	}
	details := Metadata{Description: "新的簡介", Tags: []string{" Go ", "go", "教學"}, Creators: []Creator{{Name: "原作者", URL: "https://example.com/creator"}, {Name: "協作者"}}, WorkType: "repost", SourceURL: "https://example.com/original", SourcePlatform: "B站", RepostNote: "授權說明由發布者提供", Language: "繁體中文"}
	body, _ := json.Marshal(struct {
		Title string `json:"title"`
		Metadata
	}{"new title", details})
	request := httptest.NewRequest("PUT", "http://node/api/v1/videos/"+video.ID+"/metadata", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://node")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 200 {
		store.Close()
		t.Fatalf("update failed: %d %s", response.Code, response.Body)
	}
	var updated Video
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if updated.ID != video.ID || updated.FileHash != video.FileHash || updated.Title != "new title" || len(updated.Tags) != 2 || updated.WorkType != "repost" {
		store.Close()
		t.Fatalf("bad update: %+v", updated)
	}
	// 無效更新必須保持既有來源與 Tag，不能留下部分寫入。
	if _, err := store.UpdateMetadata(context.Background(), video.ID, "bad", Metadata{WorkType: "repost"}); err == nil {
		store.Close()
		t.Fatal("invalid update accepted")
	}
	store.Close()
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved, err = store.video(video.ID)
	normalized, _ := details.normalize()
	if err != nil || saved.Title != "new title" || !reflect.DeepEqual(saved.Metadata, normalized) {
		t.Fatalf("update did not persist: %+v %v", saved, err)
	}
}
