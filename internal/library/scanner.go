package library

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ScanResult 是最近一次完整掃描的統計；離線或遺失不會刪除影片索引。
type ScanResult struct {
	StartedAt    time.Time `json:"startedAt"`
	CompletedAt  time.Time `json:"completedAt"`
	Imported     int       `json:"imported"`
	Enqueued     int       `json:"enqueued"`
	Pending      int       `json:"pending"`
	Skipped      int       `json:"skipped"`
	Changed      int       `json:"changed"`
	Deleted      int       `json:"deleted"`
	Failed       int       `json:"failed"`
	OfflinePools int       `json:"offlinePools"`
	Errors       []string  `json:"errors"`
}

// ScanStatus 提供排程設定、執行狀態與最近掃描結果。
type ScanStatus struct {
	Automatic        bool        `json:"automatic"`
	IntervalSeconds  int         `json:"intervalSeconds"`
	StableSeconds    int         `json:"stableSeconds"`
	Running          bool        `json:"running"`
	Queued           bool        `json:"queued"`
	NextRunAt        string      `json:"nextRunAt,omitempty"`
	NextRunInSeconds int         `json:"nextRunInSeconds"`
	Last             *ScanResult `json:"last"`
}

type observation struct {
	size     int64
	modified time.Time
	since    time.Time
}

// Scanner 以單一背景工作掃描儲存池；穩定性快照僅保存在記憶體。
type Scanner struct {
	store    *Store
	logger   *slog.Logger
	interval time.Duration
	stable   time.Duration
	requests chan struct{}
	observed map[string]observation
	mu       sync.Mutex
	status   ScanStatus
}

// NewScanner 建立 store 的掃描器，interval 與 stable 須為正數。
// automatic 決定是否啟用週期掃描；停用時仍可手動排程。須在提供 HTTP 服務前建立。
func NewScanner(store *Store, automatic bool, interval, stable time.Duration, logger *slog.Logger) *Scanner {
	scanner := &Scanner{store: store, logger: logger, interval: interval, stable: stable, requests: make(chan struct{}, 1), observed: map[string]observation{}, status: ScanStatus{Automatic: automatic, IntervalSeconds: int(interval / time.Second), StableSeconds: int(stable / time.Second)}}
	store.scanner = scanner
	return scanner
}

// Status 回傳狀態快照；結果完成後不再修改，供診斷 API 讀取。
func (s *Scanner) Status() ScanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.NextRunAt != "" {
		if next, err := time.Parse(time.RFC3339Nano, s.status.NextRunAt); err == nil {
			if remaining := int(time.Until(next).Seconds()); remaining > 0 {
				s.status.NextRunInSeconds = remaining
			}
		}
	}
	return s.status
}

// Request 合併重複的掃描請求，回傳是否成功加入佇列。
func (s *Scanner) Request() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Queued || s.status.Running {
		return false
	}
	select {
	case s.requests <- struct{}{}:
		s.status.Queued = true
		return true
	default:
		return false
	}
}

// Run 執行週期與手動掃描直到 ctx 取消；不可同時呼叫兩次。
func (s *Scanner) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	if s.status.Automatic {
		s.Request()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.status.Automatic {
				continue
			}
		case <-s.requests:
		}
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		select {
		case <-s.requests:
		default:
		}
		s.status.Queued = false
		s.status.Running = true
		s.mu.Unlock()
		result := s.scan(ctx, time.Now())
		result.CompletedAt = time.Now()
		s.mu.Lock()
		s.status.Last = &result
		s.status.Running = false
		if s.status.Automatic {
			s.status.NextRunAt = time.Now().Add(s.interval).UTC().Format(time.RFC3339Nano)
		} else {
			s.status.NextRunAt = ""
			s.status.NextRunInSeconds = 0
		}
		s.mu.Unlock()
		s.logger.Info("儲存池掃描完成", "enqueued", result.Enqueued, "imported", result.Imported, "pending", result.Pending, "changed", result.Changed, "deleted", result.Deleted, "failed", result.Failed, "offlinePools", result.OfflinePools)
	}
}

func (r *ScanResult) failure(err error) {
	r.Failed++
	if len(r.Errors) < 20 {
		r.Errors = append(r.Errors, err.Error())
	}
}

func supportedVideo(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".webm", ".mkv", ".mov", ".avi", ".m4v":
		return true
	default:
		return false
	}
}

func fileKey(poolID, path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return poolID + ":" + path
}

type catalogVideo struct {
	poolID   string
	size     int64
	modified int64
}

// catalog 讀取已索引影片的中繼資料與匯入工作路徑；不讀取影片內容。
func (s *Scanner) catalog(ctx context.Context) (map[string]catalogVideo, map[string]bool, error) {
	videos := map[string]catalogVideo{}
	rows, err := s.store.db.QueryContext(ctx, "SELECT pool_id,relative_path,size,modified_at FROM videos")
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var poolID, relative string
		var size, modified int64
		if err := rows.Scan(&poolID, &relative, &size, &modified); err != nil {
			rows.Close()
			return nil, nil, err
		}
		videos[fileKey(poolID, relative)] = catalogVideo{poolID: poolID, size: size, modified: modified}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	jobs := map[string]bool{}
	if s.store.queue != nil {
		rows, err := s.store.db.QueryContext(ctx, "SELECT pool_id,relative_path FROM import_jobs")
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			var poolID, relative string
			if err := rows.Scan(&poolID, &relative); err != nil {
				rows.Close()
				return nil, nil, err
			}
			jobs[fileKey(poolID, relative)] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, nil, err
		}
		rows.Close()
	}
	return videos, jobs, nil
}

func (s *Scanner) scan(ctx context.Context, now time.Time) ScanResult {
	result := ScanResult{StartedAt: now, Errors: []string{}}
	pools, err := s.store.Pools()
	if err != nil {
		result.failure(err)
		return result
	}
	catalog, importJobs, err := s.catalog(ctx)
	if err != nil {
		result.failure(err)
		return result
	}
	next := map[string]observation{}
	for _, pool := range pools {
		if ctx.Err() != nil {
			break
		}
		if !pool.Enabled {
			continue
		}
		if !pool.Online {
			result.OfflinePools++
			continue
		}
		root, err := s.store.poolPath(pool.ID)
		if err != nil {
			result.failure(err)
			continue
		}
		seen := map[string]bool{}
		complete := true
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				result.failure(walkErr)
				return nil
			}
			// 不跟隨符號連結，避免自動掃描越過管理員註冊的池邊界。
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !supportedVideo(path) {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				result.failure(err)
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				result.failure(err)
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			key := fileKey(pool.ID, relative)
			seen[key] = true
			if video, exists := catalog[key]; exists {
				result.Skipped++
				if info.Size() != video.size || info.ModTime().UnixNano() != video.modified {
					result.Changed++
				}
				return nil
			}
			if importJobs[key] {
				result.Skipped++
				return nil
			}
			previous, exists := s.observed[key]
			current := observation{size: info.Size(), modified: info.ModTime(), since: now}
			if exists && previous.size == current.size && previous.modified.Equal(current.modified) {
				current.since = previous.since
			}
			next[key] = current
			if info.Size() == 0 || now.Sub(current.since) < s.stable || now.Sub(current.modified) < s.stable {
				result.Pending++
				return nil
			}
			if s.store.queue != nil {
				_, added, err := s.store.queue.Enqueue(ctx, pool.ID, relative, "", "scan", Metadata{})
				if err != nil && err != ErrImportExists {
					result.failure(err)
				} else if added {
					result.Enqueued++
				} else {
					result.Skipped++
				}
				delete(next, key)
				return nil
			}
			if _, err := s.store.Import(ctx, pool.ID, relative, ""); err != nil {
				// 手動匯入可能在掃描期間完成；唯一索引保證只有一筆且不覆寫描述。
				var count int
				queryErr := s.store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM videos WHERE pool_id=? AND relative_path=?", pool.ID, relative).Scan(&count)
				if queryErr == nil && count > 0 {
					result.Skipped++
				} else {
					result.failure(err)
				}
				return nil
			}
			delete(next, key)
			result.Imported++
			return nil
		})
		if err != nil {
			result.failure(err)
			complete = false
		}
		if complete {
			for key, video := range catalog {
				if video.poolID == pool.ID && !seen[key] {
					result.Deleted++
				}
			}
		}
	}
	s.observed = next
	return result
}
