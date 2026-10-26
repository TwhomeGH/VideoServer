package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestVideoQueryPaginationFiltersAndOffline(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	root := t.TempDir()
	pool, err := s.AddPool("查詢影片庫", root)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 27 {
		name := fmt.Sprintf("%02d.mp4", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		metadata := Metadata{Category: "生活", Tags: []string{"旅行"}}
		if i == 0 {
			metadata = Metadata{Category: "教育", Tags: []string{"Go", "教學"}, Adult: true, Description: "中文內容與 100%_literal"}
		}
		if _, err := s.Import(context.Background(), pool.ID, name, fmt.Sprintf("影片 %02d", i), metadata); err != nil {
			t.Fatal(err)
		}
	}
	query := VideoQuery{Page: 1, PageSize: 24, Sort: "title"}
	first, err := s.QueryVideos(context.Background(), query)
	if err != nil || len(first.Videos) != 24 || first.Total != 27 || first.TotalVideos != 27 {
		t.Fatalf("first page: %+v, %v", first, err)
	}
	query.Page = 2
	second, err := s.QueryVideos(context.Background(), query)
	if err != nil || len(second.Videos) != 3 || second.Videos[0].Title != "影片 24" {
		t.Fatalf("second page: %+v, %v", second, err)
	}
	seen := map[string]bool{}
	for _, video := range append(first.Videos, second.Videos...) {
		if seen[video.ID] {
			t.Fatal("duplicate across pages")
		}
		seen[video.ID] = true
	}
	query = VideoQuery{Page: 999, PageSize: 12, Search: "100%_literal", Tag: "GO", Category: "教育", PoolID: pool.ID, Adult: "true"}
	filtered, err := s.QueryVideos(context.Background(), query)
	if err != nil || filtered.Total != 1 || filtered.Page != 1 || filtered.Videos[0].Title != "影片 00" {
		t.Fatalf("combined filters: %+v, %v", filtered, err)
	}
	if err := os.Rename(filepath.Join(root, "00.mp4"), filepath.Join(root, "00.offline")); err != nil {
		t.Fatal(err)
	}
	filtered, err = s.QueryVideos(context.Background(), query)
	if err != nil || filtered.Total != 1 || filtered.Videos[0].Available {
		t.Fatalf("offline record: %+v, %v", filtered, err)
	}
	query.Search = "' OR 1=1 --"
	empty, err := s.QueryVideos(context.Background(), query)
	if err != nil || empty.Total != 0 || empty.Videos == nil {
		t.Fatalf("literal query: %+v, %v", empty, err)
	}
	query = VideoQuery{Page: 1, PageSize: 24, Adult: "false"}
	unmarked, err := s.QueryVideos(context.Background(), query)
	if err != nil || unmarked.Total != 26 {
		t.Fatalf("unmarked: %+v, %v", unmarked, err)
	}
	mux := http.NewServeMux()
	s.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/library?page=2&pageSize=24&sort=title", nil))
	var body struct {
		Videos     []Video                                          `json:"videos"`
		Pagination struct{ Total, Page, PageSize, TotalVideos int } `json:"pagination"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || len(body.Videos) != 3 || body.Pagination.Total != 27 || body.Pagination.Page != 2 {
		t.Fatalf("API response: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/library?pageSize=101", nil))
	if response.Code != 400 {
		t.Fatal("API accepted oversized page")
	}
}

func TestVideoQueryValidation(t *testing.T) {
	for _, raw := range []string{"page=0", "page=-1", "page=bad", "page=1000001", "page=1&page=2", "pageSize=0", "pageSize=101", "adult=yes", "sort=title%3BDROP", "q=" + url.QueryEscape(string(make([]byte, 201)))} {
		values, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseVideoQuery(values); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	query, err := parseVideoQuery(url.Values{})
	if err != nil || query.Page != 1 || query.PageSize != 24 {
		t.Fatalf("default: %+v, %v", query, err)
	}
}
