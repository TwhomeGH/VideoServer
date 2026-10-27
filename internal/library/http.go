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
	"strconv"
	"strings"
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
// 登入授權由 runtime 的 auth middleware 統一執行；寫入另檢查 Origin。
func safeDiscoveredPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func (s *Store) serveDiscoveredContent(w http.ResponseWriter, r *http.Request) {
	poolID := strings.TrimSpace(r.URL.Query().Get("poolId"))
	relative := strings.TrimSpace(r.URL.Query().Get("path"))
	if !safeDiscoveredPath(relative) {
		failure(w, 400, "影片路徑無效")
		return
	}
	pool, err := s.poolByID(poolID)
	if err != nil {
		failure(w, 404, "找不到儲存池")
		return
	}
	if pool.Provider != "webdav" {
		failure(w, 409, "此儲存池不支援遠端播放")
		return
	}
	if !pool.Enabled {
		failure(w, 409, "儲存池已停用")
		return
	}
	exists, err := s.DiscoveredExists(r.Context(), poolID, relative)
	if err != nil {
		failure(w, 500, "無法檢查找到的影片")
		return
	}
	if !exists {
		failure(w, 404, "找不到找到的影片")
		return
	}
	s.proxyWebDAV(w, r, pool, relative)
}

func (s *Store) proxyWebDAV(w http.ResponseWriter, r *http.Request, pool Pool, relative string) {
	username, password, err := s.WebDAVCredentials(pool.ID)
	if err != nil {
		failure(w, 500, "無法讀取 WebDAV 憑證")
		return
	}
	target, err := webDAVTarget(pool, relative)
	if err != nil {
		failure(w, 500, "WebDAV URL 無效")
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), nil)
	if err != nil {
		failure(w, 500, "無法建立遠端請求")
		return
	}
	request.SetBasicAuth(username, password)
	for _, header := range []string{"Range", "If-Range"} {
		if value := r.Header.Get(header); value != "" {
			request.Header.Set(header, value)
		}
	}
	response, err := s.webDAVHTTPClient(pool, 0).Do(request)
	if err != nil {
		failure(w, 502, "無法連線 WebDAV")
		return
	}
	defer response.Body.Close()
	for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, response.Body)
	}
}

func (s *Store) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/schedule", func(w http.ResponseWriter, r *http.Request) {
		var scan, backfill, jobs any
		if s.scanner != nil {
			scan = s.scanner.Status()
		}
		if s.queue != nil {
			backfill, _ = s.queue.BackfillStatus(r.Context())
			jobs, _ = s.queue.WorkStatus(r.Context())
		}
		respond(w, 200, map[string]any{"scan": scan, "backfill": backfill, "jobs": jobs})
	})
	mux.HandleFunc("GET /api/v1/library", func(w http.ResponseWriter, r *http.Request) {
		values, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			failure(w, 400, "查詢參數格式無效")
			return
		}
		query, err := parseVideoQuery(values)
		if err != nil {
			failure(w, 400, err.Error())
			return
		}
		pools, err := s.Pools()
		if err != nil {
			failure(w, 500, "無法讀取儲存池")
			return
		}
		videoPage, err := s.QueryVideos(r.Context(), query)
		if err != nil {
			failure(w, 500, "無法讀取影片索引")
			return
		}
		var scan any
		if s.scanner != nil {
			scan = s.scanner.Status()
		}
		jobs := []ImportJob{}
		if s.queue != nil {
			jobs, err = s.queue.Jobs()
			if err != nil {
				failure(w, 500, "無法讀取匯入工作")
				return
			}
		}
		respond(w, 200, map[string]any{"nodeId": s.nodeID, "pools": pools, "videos": videoPage.Videos, "pagination": map[string]int{"page": videoPage.Page, "pageSize": videoPage.PageSize, "total": videoPage.Total, "totalVideos": videoPage.TotalVideos}, "scan": scan, "jobs": jobs})
	})
	mux.HandleFunc("POST /api/v1/pools", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Name          string `json:"name"`
			Path          string `json:"path"`
			Provider      string `json:"provider"`
			URL           string `json:"url"`
			Username      string `json:"username"`
			Password      string `json:"password"`
			Root          string `json:"root"`
			TLSSkipVerify bool   `json:"tlsSkipVerify"`
			TLSCA         string `json:"tlsCa"`
		}
		if !decode(w, r, &request) {
			return
		}
		var pool Pool
		var err error
		if request.Provider == "webdav" || (request.Provider == "" && request.URL != "") {
			pool, err = s.AddRemotePool(request.Name, request.URL, request.Username, request.Password, request.Root, request.TLSCA, request.TLSSkipVerify)
		} else {
			pool, err = s.AddPool(request.Name, request.Path)
		}
		if err != nil {
			failure(w, 400, "無法註冊儲存池："+err.Error())
			return
		}
		respond(w, 201, pool)
		if s.scanner != nil && s.scanner.Status().Automatic {
			s.scanner.Request()
		}
	})
	mux.HandleFunc("POST /api/v1/pools/refresh", func(w http.ResponseWriter, r *http.Request) {
		var request struct{}
		if !decode(w, r, &request) {
			return
		}
		pools, err := s.RefreshPoolMetadata()
		if err != nil {
			failure(w, 500, "無法重新檢查儲存池資訊")
			return
		}
		respond(w, 200, pools)
	})
	if s.queue != nil {
		mux.HandleFunc("POST /api/v1/backfill/usage/reset", func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				Confirm bool `json:"confirm"`
			}
			if !decode(w, r, &request) {
				return
			}
			if !request.Confirm {
				failure(w, 400, "重設今日用量需要 confirm=true")
				return
			}
			status, err := s.queue.ResetBackfillUsage(r.Context())
			if err != nil {
				failure(w, 500, "無法重設自動補齊用量")
				return
			}
			respond(w, 200, status)
		})
	}
	mux.HandleFunc("PATCH /api/v1/pools/{id}", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Enabled           *bool   `json:"enabled"`
			ScanExcludeSystem *bool   `json:"scanExcludeSystem"`
			IndexOnly         *bool   `json:"indexOnly"`
			TLSCA             *string `json:"tlsCa"`
			TLSSkipVerify     *bool   `json:"tlsSkipVerify"`
		}
		if !decode(w, r, &request) {
			return
		}
		if request.Enabled == nil && request.ScanExcludeSystem == nil && request.IndexOnly == nil && request.TLSCA == nil && request.TLSSkipVerify == nil {
			failure(w, 400, "請提供 enabled、scanExcludeSystem 或 indexOnly")
			return
		}
		id := r.PathValue("id")
		var pool Pool
		var err error
		if request.Enabled != nil {
			pool, err = s.SetPoolEnabled(id, *request.Enabled)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					failure(w, 404, "找不到儲存池")
				} else {
					failure(w, 500, "無法更新儲存池狀態")
				}
				return
			}
		}
		if request.ScanExcludeSystem != nil {
			pool, err = s.SetPoolScanExcludeSystem(id, *request.ScanExcludeSystem)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					failure(w, 404, "找不到儲存池")
				} else {
					failure(w, 500, "無法更新掃描政策")
				}
				return
			}
		}
		if request.TLSCA != nil || request.TLSSkipVerify != nil {
			current, lookupErr := s.poolByID(id)
			if lookupErr != nil {
				failure(w, 404, "找不到儲存池")
				return
			}
			ca := current.TLSCA
			if request.TLSCA != nil {
				ca = *request.TLSCA
			}
			skip := current.TLSSkipVerify
			if request.TLSSkipVerify != nil {
				skip = *request.TLSSkipVerify
			}
			pool, err = s.SetPoolTLS(id, ca, skip)
			if err != nil {
				failure(w, 400, err.Error())
				return
			}
		}
		if request.IndexOnly != nil {
			current, lookupErr := s.poolByID(id)
			if lookupErr != nil {
				failure(w, 404, "找不到儲存池")
				return
			}
			if current.Provider == "webdav" && !*request.IndexOnly {
				failure(w, 400, "WebDAV 池目前僅支援 index-only")
				return
			}
			pool, err = s.SetPoolIndexOnly(id, *request.IndexOnly)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					failure(w, 404, "找不到儲存池")
				} else {
					failure(w, 500, "無法更新匯入模式")
				}
				return
			}
		}
		s.publish("scan")
		respond(w, 200, pool)
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
		if s.queue == nil {
			failure(w, 503, "背景匯入器尚未啟用")
			return
		}
		job, _, err := s.queue.Enqueue(r.Context(), request.PoolID, request.Path, request.Title, "manual", request.Metadata)
		if err != nil {
			if errors.Is(err, ErrImportExists) {
				failure(w, 409, err.Error())
				return
			}
			if errors.Is(err, ErrPoolDisabled) || errors.Is(err, ErrPoolOffline) {
				failure(w, 409, err.Error())
				return
			}
			failure(w, 400, "匯入失敗：請確認影片在儲存池內、檔案可讀且尚未匯入")
			return
		}
		respond(w, 202, job)
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
	if s.scanner != nil {
		mux.HandleFunc("POST /api/v1/library/scan", func(w http.ResponseWriter, r *http.Request) {
			var request struct{}
			if !decode(w, r, &request) {
				return
			}
			s.scanner.Request()
			respond(w, 202, s.scanner.Status())
		})
		mux.HandleFunc("POST /api/v1/pools/{id}/scan", func(w http.ResponseWriter, r *http.Request) {
			var request struct{}
			if !decode(w, r, &request) {
				return
			}
			pool, err := s.poolByID(r.PathValue("id"))
			if errors.Is(err, sql.ErrNoRows) {
				failure(w, 404, "找不到儲存池")
				return
			}
			if err != nil {
				failure(w, 500, "無法讀取儲存池")
				return
			}
			if !pool.Enabled {
				failure(w, 409, "儲存池已停用")
				return
			}
			if !pool.Online {
				failure(w, 409, "儲存池目前離線")
				return
			}
			if !s.scanner.RequestPool(pool.ID) {
				failure(w, 409, "掃描已在排程或執行中")
				return
			}
			respond(w, 202, s.scanner.Status())
		})
	}
	if s.queue != nil {

		mux.HandleFunc("GET /api/v1/discovered/content", s.serveDiscoveredContent)
		mux.HandleFunc("HEAD /api/v1/discovered/content", s.serveDiscoveredContent)
		mux.HandleFunc("GET /api/v1/discovered", func(w http.ResponseWriter, r *http.Request) {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
			result, err := s.ListDiscovered(r.Context(), r.URL.Query().Get("poolId"), r.URL.Query().Get("q"), page, pageSize)
			if err != nil {
				failure(w, 500, "無法讀取找到的影片")
				return
			}
			respond(w, 200, result)
		})
		mux.HandleFunc("POST /api/v1/discovered/import", func(w http.ResponseWriter, r *http.Request) {
			if s.queue == nil {
				failure(w, 503, "匯入佇列尚未就緒")
				return
			}
			var request struct {
				Items []struct {
					PoolID       string `json:"poolId"`
					RelativePath string `json:"relativePath"`
				} `json:"items"`
			}
			if !decode(w, r, &request) {
				return
			}
			if len(request.Items) == 0 || len(request.Items) > 100 {
				failure(w, 400, "一次需匯入 1 到 100 個項目")
				return
			}
			jobs := []ImportJob{}
			errorsList := []string{}
			imported := 0
			for _, item := range request.Items {
				pool, lookupErr := s.poolByID(item.PoolID)
				if lookupErr != nil {
					errorsList = append(errorsList, "找不到儲存池")
					continue
				}
				if pool.Provider == "webdav" {
					if _, err := s.ImportRemoteVideo(r.Context(), item.PoolID, item.RelativePath, "", Metadata{}); err != nil {
						errorsList = append(errorsList, err.Error())
					} else {
						imported++
					}
					continue
				}
				job, added, err := s.queue.Enqueue(r.Context(), item.PoolID, item.RelativePath, "", "manual", Metadata{})
				if err != nil {
					errorsList = append(errorsList, err.Error())
					continue
				}
				if added {
					jobs = append(jobs, job)
					imported++
				}
			}
			respond(w, 200, map[string]any{"imported": imported, "jobs": jobs, "errors": errorsList})
		})
		mux.HandleFunc("GET /api/v1/import-jobs", func(w http.ResponseWriter, r *http.Request) {
			jobs, err := s.queue.Jobs()
			if err != nil {
				failure(w, 500, "無法讀取背景工作")
				return
			}
			respond(w, 200, jobs)
		})
		mux.HandleFunc("POST /api/v1/videos/{id}/probe", func(w http.ResponseWriter, r *http.Request) {
			var request struct{}
			if !decode(w, r, &request) {
				return
			}
			job, err := s.queue.EnqueueProbe(r.Context(), r.PathValue("id"))
			if errors.Is(err, sql.ErrNoRows) {
				failure(w, 404, "找不到影片")
				return
			}
			if errors.Is(err, ErrPoolDisabled) || errors.Is(err, ErrPoolOffline) {
				failure(w, 409, err.Error())
				return
			}
			if err != nil {
				failure(w, 500, "無法排程影片資訊檢查")
				return
			}
			respond(w, 202, job)
		})

		mux.HandleFunc("POST /api/v1/videos/{id}/verify", func(w http.ResponseWriter, r *http.Request) {
			var request struct{}
			if !decode(w, r, &request) {
				return
			}
			if s.queue == nil {
				failure(w, 503, "匯入佇列尚未就緒")
				return
			}
			job, err := s.queue.EnqueueVerify(r.Context(), r.PathValue("id"))
			if errors.Is(err, sql.ErrNoRows) {
				failure(w, 404, "找不到影片")
				return
			}
			if err != nil {
				failure(w, 409, err.Error())
				return
			}
			respond(w, 202, job)
		})
		mux.HandleFunc("POST /api/v1/videos/{id}/verify/cancel", func(w http.ResponseWriter, r *http.Request) {
			var request struct{}
			if !decode(w, r, &request) {
				return
			}
			if s.queue == nil {
				failure(w, 503, "匯入佇列尚未就緒")
				return
			}
			job, err := s.queue.CancelVerify(r.Context(), r.PathValue("id"))
			if errors.Is(err, sql.ErrNoRows) {
				failure(w, 404, "找不到進行中的驗證")
				return
			}
			if err != nil {
				failure(w, 500, "無法取消驗證")
				return
			}
			respond(w, 202, job)
		})
		mux.HandleFunc("GET /api/v1/import-jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
			job, err := s.queue.Job(r.PathValue("id"))
			if errors.Is(err, sql.ErrNoRows) {
				failure(w, 404, "找不到匯入工作")
				return
			}
			if err != nil {
				failure(w, 500, "無法讀取匯入工作")
				return
			}
			respond(w, 200, job)
		})
		mux.HandleFunc("POST /api/v1/import-jobs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
			var request struct{}
			if !decode(w, r, &request) {
				return
			}
			job, err := s.queue.Retry(r.Context(), r.PathValue("id"))
			if err != nil {
				failure(w, 409, err.Error())
				return
			}
			respond(w, 202, job)
		})
	}
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
	if video.Provider == "webdav" {
		pool, err := s.poolByID(video.PoolID)
		if err != nil {
			failure(w, 404, "找不到儲存池")
			return
		}
		if !pool.Enabled {
			failure(w, 409, "所屬儲存池已停用")
			return
		}
		s.proxyWebDAV(w, r, pool, video.relativePath)
		return
	}
	file, err := s.openVideo(video)
	if err != nil {
		switch {
		case errors.Is(err, ErrPoolDisabled):
			failure(w, 409, "所屬儲存池已停用")
		case errors.Is(err, ErrPoolOffline):
			failure(w, 503, "所屬儲存池目前離線")
		case errors.Is(err, ErrSourceChanged):
			failure(w, 503, "來源檔案已變更")
		default:
			failure(w, 503, "來源檔案遺失或無法讀取")
		}
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
