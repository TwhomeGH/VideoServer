package library

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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

type PoolScanStatus struct {
	PoolID           string `json:"poolId"`
	Name             string `json:"name,omitempty"`
	Tier             string `json:"tier"`
	BackoffSeconds   int    `json:"backoffSeconds"`
	Quiet            int    `json:"quiet"`
	LastActivityAt   string `json:"lastActivityAt,omitempty"`
	NextRunAt        string `json:"nextRunAt,omitempty"`
	NextRunInSeconds int    `json:"nextRunInSeconds"`
}

// ScanStatus 提供排程設定、執行狀態、每池退避與最近掃描結果。
type ScanStatus struct {
	Automatic        bool             `json:"automatic"`
	IntervalSeconds  int              `json:"intervalSeconds"`
	IdleMaxSeconds   int              `json:"idleMaxSeconds"`
	StableSeconds    int              `json:"stableSeconds"`
	Running          bool             `json:"running"`
	Queued           bool             `json:"queued"`
	NextRunAt        string           `json:"nextRunAt,omitempty"`
	NextRunInSeconds int              `json:"nextRunInSeconds"`
	Pools            []PoolScanStatus `json:"pools,omitempty"`
	Last             *ScanResult      `json:"last"`
}

type observation struct {
	size     int64
	modified time.Time
	since    time.Time
}

type poolScanState struct {
	name         string
	tier         string
	lastActivity time.Time
	next         time.Time
	interval     time.Duration
}

// Scanner 以單一背景工作掃描儲存池；每個池有自己的下次執行時間與退避。
type Scanner struct {
	store     *Store
	logger    *slog.Logger
	interval  time.Duration
	idleMax   time.Duration
	stable    time.Duration
	requests  chan struct{}
	observed  map[string]observation
	states    map[string]poolScanState
	forceAll  bool
	forcePool map[string]bool
	mu        sync.Mutex
	status    ScanStatus
}

// NewScanner 建立 store 的掃描器；interval 是活躍池基礎間隔，idleMax 是閒置上限。
// automatic 決定是否啟用週期掃描；停用時仍可手動排程。須在提供 HTTP 服務前建立。
func NewScanner(store *Store, automatic bool, interval, stable time.Duration, logger *slog.Logger) *Scanner {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if stable < 0 {
		stable = 0
	}
	scanner := &Scanner{
		store: store, logger: logger, interval: interval, idleMax: 6 * time.Hour,
		stable: stable, requests: make(chan struct{}, 1), observed: map[string]observation{},
		states: map[string]poolScanState{}, forcePool: map[string]bool{},
		status: ScanStatus{Automatic: automatic, IntervalSeconds: int(interval / time.Second), IdleMaxSeconds: 21600, StableSeconds: int(stable / time.Second)},
	}
	store.scanner = scanner
	return scanner
}

// SetIdleMax 設定閒置池的掃描退避上限。
func (s *Scanner) SetIdleMax(max time.Duration) {
	if max <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if max < s.interval {
		max = s.interval
	}
	s.idleMax = max
	s.status.IdleMaxSeconds = int(max / time.Second)
}

// Status 回傳掃描與每池退避快照。
func (s *Scanner) Status() ScanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	status.Pools = make([]PoolScanStatus, 0, len(s.states))
	for id, state := range s.states {
		remaining := int(time.Until(state.next).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		status.Pools = append(status.Pools, PoolScanStatus{
			PoolID: id, Name: state.name, Tier: state.tier, BackoffSeconds: int(state.interval / time.Second),
			LastActivityAt: state.lastActivity.UTC().Format(time.RFC3339Nano),
			NextRunAt:      state.next.UTC().Format(time.RFC3339Nano), NextRunInSeconds: remaining,
		})
	}
	sort.Slice(status.Pools, func(i, j int) bool { return status.Pools[i].PoolID < status.Pools[j].PoolID })
	if status.NextRunAt != "" {
		if next, err := time.Parse(time.RFC3339Nano, status.NextRunAt); err == nil {
			if remaining := int(time.Until(next).Seconds()); remaining > 0 {
				status.NextRunInSeconds = remaining
			}
		}
	}
	return status
}

// Request 要求掃描所有池；已有排程或執行中的掃描時回傳 false。
func (s *Scanner) Request() bool {
	s.mu.Lock()
	if s.status.Queued || s.status.Running {
		s.mu.Unlock()
		return false
	}
	s.status.Queued = true
	s.forceAll = true
	s.mu.Unlock()
	select {
	case s.requests <- struct{}{}:
	default:
	}
	return true
}

// RequestPool 只要求掃描一個池，完成後重置該池退避。
func (s *Scanner) RequestPool(id string) bool {
	if id == "" {
		return false
	}
	s.mu.Lock()
	if s.status.Queued || s.status.Running {
		s.mu.Unlock()
		return false
	}
	s.status.Queued = true
	s.forcePool[id] = true
	s.mu.Unlock()
	select {
	case s.requests <- struct{}{}:
	default:
	}
	return true
}

// Run 依每池 due time 掃描；短 tick 只做排程檢查，不讀取影片。
func (s *Scanner) Run(ctx context.Context) {
	tick := s.interval
	if tick > time.Second {
		tick = time.Second
	}
	if tick <= 0 {
		tick = time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	s.mu.Lock()
	automatic := s.status.Automatic
	s.mu.Unlock()
	if automatic {
		s.runDue(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.requests:
		}
		if ctx.Err() != nil {
			return
		}
		s.runDue(ctx)
	}
}

func (s *Scanner) runDue(ctx context.Context) {
	s.mu.Lock()
	forceAll := s.forceAll
	forcePool := s.forcePool
	s.forceAll = false
	s.forcePool = map[string]bool{}
	automatic := s.status.Automatic
	if !automatic && !forceAll && len(forcePool) == 0 {
		s.status.Queued = false
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	pools, err := s.store.Pools()
	if err != nil {
		s.mu.Lock()
		result := ScanResult{StartedAt: time.Now(), Errors: []string{}}
		result.failure(err)
		result.CompletedAt = time.Now()
		s.status.Last = &result
		s.status.Queued = false
		s.status.Running = false
		s.mu.Unlock()
		return
	}
	now := time.Now()
	due := map[string]bool{}
	names := map[string]string{}
	byID := map[string]Pool{}
	for _, pool := range pools {
		byID[pool.ID] = pool
		if !pool.Enabled || !pool.Online {
			continue
		}
		if forceAll || forcePool[pool.ID] {
			due[pool.ID] = true
			names[pool.ID] = pool.Name
			continue
		}
		if !automatic {
			continue
		}
		state, exists := s.states[pool.ID]
		if !exists || !now.Before(state.next) {
			due[pool.ID] = true
			names[pool.ID] = pool.Name
		}
	}
	if len(due) == 0 {
		s.mu.Lock()
		s.status.Queued = false
		s.status.Running = false
		s.updateNextLocked(pools)
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	s.status.Queued = false
	s.status.Running = true
	s.mu.Unlock()

	result, active := s.scanPools(ctx, now, pools, due)
	result.CompletedAt = time.Now()
	for _, pool := range pools {
		if pool.Enabled && !pool.Online {
			result.OfflinePools++
		}
	}

	s.mu.Lock()
	for id := range due {
		state := s.states[id]
		state.name = names[id]
		lastActivity := state.lastActivity
		if lastActivity.IsZero() {
			if parsed, err := time.Parse(time.RFC3339Nano, byID[id].LastActivityAt); err == nil {
				lastActivity = parsed
			}
		}
		if lastActivity.IsZero() {
			lastActivity = now
		}
		if forceAll || forcePool[id] || active[id] {
			lastActivity = now
			if err := s.store.touchPoolActivity(id, now); err != nil {
				s.logger.Error("更新儲存池活動時間失敗", "poolId", id, "error", err)
			}
		}
		state.tier, state.interval = scanTier(now, lastActivity, s.interval, s.idleMax)
		state.lastActivity = lastActivity
		state.next = now.Add(state.interval)
		s.states[id] = state
	}
	s.status.Last = &result
	s.status.Running = false
	s.updateNextLocked(pools)
	s.mu.Unlock()
	s.logger.Info("儲存池掃描完成", "enqueued", result.Enqueued, "imported", result.Imported, "pending", result.Pending, "changed", result.Changed, "deleted", result.Deleted, "failed", result.Failed, "offlinePools", result.OfflinePools)
}

func (s *Scanner) updateNextLocked(pools []Pool) {
	if !s.status.Automatic {
		s.status.NextRunAt = ""
		s.status.NextRunInSeconds = 0
		return
	}
	var next time.Time
	for _, pool := range pools {
		if !pool.Enabled || !pool.Online {
			continue
		}
		state, exists := s.states[pool.ID]
		if !exists {
			s.status.NextRunAt = ""
			s.status.NextRunInSeconds = 0
			return
		}
		if next.IsZero() || state.next.Before(next) {
			next = state.next
		}
	}
	if next.IsZero() {
		s.status.NextRunAt = ""
		s.status.NextRunInSeconds = 0
		return
	}
	s.status.NextRunAt = next.UTC().Format(time.RFC3339Nano)
}

func scanTier(now, lastActivity time.Time, base, idleMax time.Duration) (string, time.Duration) {
	if base <= 0 {
		base = 30 * time.Second
	}
	warm := 5 * time.Minute
	if warm < base {
		warm = base
	}
	cold := 30 * time.Minute
	if cold < base {
		cold = base
	}
	slowerWarm := 15 * time.Minute
	if slowerWarm < base {
		slowerWarm = base
	}
	dormant := idleMax
	if dormant < cold {
		dormant = cold
	}
	if lastActivity.IsZero() || now.Sub(lastActivity) < time.Hour {
		return "hot", base
	}
	if now.Sub(lastActivity) < 24*time.Hour {
		if now.Sub(lastActivity) >= 6*time.Hour {
			return "warm", slowerWarm
		}
		return "warm", warm
	}
	if now.Sub(lastActivity) < 7*24*time.Hour {
		return "cold", cold
	}
	return "dormant", dormant
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
	pools, err := s.store.Pools()
	if err != nil {
		result := ScanResult{StartedAt: now, Errors: []string{}}
		result.failure(err)
		return result
	}
	result, _ := s.scanPools(ctx, now, pools, nil)
	return result
}

func (s *Scanner) scanPools(ctx context.Context, now time.Time, pools []Pool, only map[string]bool) (ScanResult, map[string]bool) {
	result := ScanResult{StartedAt: now, Errors: []string{}}
	active := map[string]bool{}
	catalog, importJobs, err := s.catalog(ctx)
	if err != nil {
		result.failure(err)
		return result, active
	}
	next := map[string]observation{}
	for _, pool := range pools {
		if only != nil && !only[pool.ID] {
			continue
		}
		before := result
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
		if result.Imported > before.Imported || result.Enqueued > before.Enqueued || result.Pending > before.Pending ||
			result.Changed > before.Changed || result.Deleted > before.Deleted || result.Failed > before.Failed ||
			len(result.Errors) > len(before.Errors) {
			active[pool.ID] = true
		}
	}
	s.observed = next
	return result, active
}
