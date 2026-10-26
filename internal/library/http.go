package library

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
)

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func failure(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}

// decode 限制管理請求大小與來源，避免其他網站以瀏覽器跨站操作本機檔案。
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host != r.Host {
			failure(w, 403, "不允許跨站管理請求")
			return false
		}
	}
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "application/json" {
		failure(w, 415, "請使用 application/json")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		failure(w, 400, "請求 JSON 格式無效")
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		failure(w, 400, "請求必須只包含一個 JSON 物件")
		return false
	}
	return true
}

// Register 將 Video Node 的儲存池、影片匯入與 Range 播放路由加入 mux。
// 本階段管理 API 限可信任區網，跨站檢查並不取代後續登入授權。
func (s *Store) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/library", func(w http.ResponseWriter, r *http.Request) {
		pools, err := s.Pools()
		if err != nil {
			failure(w, 500, "無法讀取儲存池")
			return
		}
		videos, err := s.Videos()
		if err != nil {
			failure(w, 500, "無法讀取影片索引")
			return
		}
		respond(w, 200, map[string]any{"nodeId": s.nodeID, "pools": pools, "videos": videos})
	})
	mux.HandleFunc("POST /api/v1/pools", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if !decode(w, r, &request) {
			return
		}
		pool, err := s.AddPool(request.Name, request.Path)
		if err != nil {
			failure(w, 400, "無法註冊目錄：請確認路徑、權限與是否已註冊")
			return
		}
		respond(w, 201, pool)
	})
	mux.HandleFunc("POST /api/v1/videos/import", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			PoolID string `json:"poolId"`
			Path   string `json:"path"`
			Title  string `json:"title"`
			Metadata
		}
		if !decode(w, r, &request) {
			return
		}
		if _, err := request.Metadata.normalize(); err != nil {
			failure(w, 400, err.Error())
			return
		}
		video, err := s.Import(r.Context(), request.PoolID, request.Path, request.Title, request.Metadata)
		if err != nil {
			failure(w, 400, "匯入失敗：請確認影片在儲存池內、檔案可讀且尚未匯入")
			return
		}
		respond(w, 201, video)
	})
	mux.HandleFunc("PUT /api/v1/videos/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Title string `json:"title"`
			Metadata
		}
		if !decode(w, r, &request) {
			return
		}
		if _, err := request.Metadata.normalize(); err != nil {
			failure(w, 400, err.Error())
			return
		}
		video, err := s.UpdateMetadata(r.Context(), r.PathValue("id"), request.Title, request.Metadata)
		if errors.Is(err, sql.ErrNoRows) {
			failure(w, 404, "找不到影片")
			return
		}
		if err != nil {
			failure(w, 400, "更新失敗，請確認標題與描述欄位")
			return
		}
		respond(w, 200, video)
	})
	mux.HandleFunc("GET /api/v1/videos/{id}/content", s.play)
}

func (s *Store) play(w http.ResponseWriter, r *http.Request) {
	video, err := s.video(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		failure(w, 404, "找不到影片")
		return
	}
	if err != nil {
		failure(w, 500, "無法讀取影片索引")
		return
	}
	file, err := s.openVideo(video)
	if err != nil {
		failure(w, 503, "來源離線或已變更")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		failure(w, 503, "來源不可用")
		return
	}
	w.Header().Set("ETag", `"`+video.FileHash+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(video.relativePath), info.ModTime(), file)
}
