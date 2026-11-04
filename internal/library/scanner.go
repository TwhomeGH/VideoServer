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
	Renamed      int       `json:"renamed"`
	Failed       int       `json:"failed"`
	Discovered   int       `json:"discovered"`
	OfflinePools int       `json:"offlinePools"`
	Errors       []string  `json:"errors"`
}

type PoolScanStatus struct {
	PoolID             string `json:"poolId"`
	Name               string `json:"name,omitempty"`
	Tier               string `json:"tier"`
	BackoffSeconds     int    `json:"backoffSeconds"`
	Quiet              int    `json:"quiet"`
	LastActivityAt     string `json:"lastActivityAt,omitempty"`
	NextBackoffAt      string `json:"nextBackoffAt,omitempty"`
	NextBackoffSeconds int    `json:"nextBackoffSeconds,omitempty"`
	NextRunAt          string `json:"nextRunAt,omitempty"`
	NextRunInSeconds   int    `json:"nextRunInSeconds"`
}

// ScanStatus 提供排程設定、執行狀態、每池退避與最近掃描結果。
type ScanStatus struct {
	Automatic        bool             `json:"automatic"`
	IntervalSeconds  int              `json:"intervalSeconds"`
	WarmSeconds      int              `json:"warmSeconds"`
	WarmSlowSeconds  int              `json:"warmSlowSeconds"`
	ColdSeconds      int              `json:"coldSeconds"`
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

type scanActivity int

const (
	activityNone scanActivity = iota
	activityPromote
	activityHot
)

type poolScanState struct {
	name         string
	tier         string
	lastActivity time.Time
	scanActivity time.Time
	next         time.Time
	interval     time.Duration
}

// Scanner 以單一背景工作掃描儲存池；每個池有自己的下次執行時間與退避。
type Scanner struct {
	store     *Store
	logger    *slog.Logger
	interval  time.Duration
	warm      time.Duration
	warmSlow  time.Duration
	cold      time.Duration
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
		store: store, logger: logger, interval: interval, warm: 5 * time.Minute, warmSlow: 15 * time.Minute, cold: time.Hour, idleMax: 24 * time.Hour,
		stable: stable, requests: make(chan struct{}, 1), observed: map[string]observation{},
		states: map[string]poolScanState{}, forcePool: map[string]bool{},
		status: ScanStatus{Automatic: automatic, IntervalSeconds: int(interval / time.Second), WarmSeconds: 300, WarmSlowSeconds: 900, ColdSeconds: 3600, IdleMaxSeconds: 86400, StableSeconds: int(stable / time.Second)},
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
	now := time.Now()
	for id, state := range s.states {
		remaining := max(int(time.Until(state.next).Seconds()), 0)
		scanActivity := state.scanActivity
		if scanActivity.IsZero() {
			scanActivity = state.lastActivity
		}
		nextBackoffAt, nextBackoff, hasNextBackoff := s.nextBackoff(now, scanActivity, state.tier)
		item := PoolScanStatus{
			PoolID: id, Name: state.name, Tier: state.tier, BackoffSeconds: int(state.interval / time.Second),
			LastActivityAt: state.lastActivity.UTC().Format(time.RFC3339Nano),
			NextRunAt:      state.next.UTC().Format(time.RFC3339Nano), NextRunInSeconds: remaining,
		}
		if hasNextBackoff {
			item.NextBackoffAt = nextBackoffAt.UTC().Format(time.RFC3339Nano)
			item.NextBackoffSeconds = int(nextBackoff / time.Second)
		}
		status.Pools = append(status.Pools, item)
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

// SetTierIntervals 設定暖、暖後段與冷池間隔；較小值會被提升到熱池間隔。
func (s *Scanner) SetTierIntervals(warm, warmSlow, cold time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if warm < s.interval {
		warm = s.interval
	}
	if warmSlow < warm {
		warmSlow = warm
	}
	if cold < warmSlow {
		cold = warmSlow
	}
	s.warm, s.warmSlow, s.cold = warm, warmSlow, cold
	s.status.WarmSeconds = int(warm / time.Second)
	s.status.WarmSlowSeconds = int(warmSlow / time.Second)
	s.status.ColdSeconds = int(cold / time.Second)
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

// NotifyChange 由 watcher 呼叫，將指定池排入下一輪掃描；掃描仍是權威來源。
func (s *Scanner) NotifyChange(poolID string) {
	if poolID == "" {
		return
	}
	s.mu.Lock()
	s.forcePool[poolID] = true
	s.mu.Unlock()
	select {
	case s.requests <- struct{}{}:
	default:
	}
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
	s.store.publish("scan")

	result, activity := s.scanPools(ctx, now, pools, due)
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
		scanActivity := state.scanActivity
		if scanActivity.IsZero() {
			scanActivity = lastActivity
		}
		act := activity[id]
		if forceAll || forcePool[id] {
			act = activityHot
		}
		previousTier, previousInterval := state.tier, state.interval
		promoted := false
		switch act {
		case activityHot:
			// 新檔、待穩定檔、手動掃描與失敗：回到熱，讓後續掃描盡快跟進。
			lastActivity = now
			scanActivity = now
			if err := s.store.touchPoolActivity(id, now); err != nil {
				s.logger.Error("更新儲存池活動時間失敗", "poolId", id, "error", err)
			}
		case activityPromote:
			// 變更／刪除只上升一級；lastActivity 記真實時間，掃描時鐘不變。
			lastActivity = now
			state.tier, state.interval = s.promotedTier(previousTier, previousInterval)
			promoted = true
			if err := s.store.touchPoolActivity(id, now); err != nil {
				s.logger.Error("更新儲存池活動時間失敗", "poolId", id, "error", err)
			}
		}
		if !promoted {
			state.tier, state.interval = scanTier(now, scanActivity, s.interval, s.warm, s.warmSlow, s.cold, s.idleMax)
		}
		if byID[id].Provider == "webdav" && state.interval < 5*time.Minute {
			state.interval = 5 * time.Minute
		}
		state.lastActivity = lastActivity
		state.scanActivity = scanActivity
		state.next = now.Add(state.interval)
		s.states[id] = state
	}
	s.status.Last = &result
	s.status.Running = false
	s.updateNextLocked(pools)
	s.mu.Unlock()
	s.store.publish("scan")
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

var systemExcludedNames = map[string]bool{
	"windows": true, "program files": true, "program files (x86)": true,
	"programdata": true, "$recycle.bin": true, "system volume information": true,
	"recovery": true, "perflogs": true, "node_modules": true, ".git": true,
	".svn": true, ".hg": true, "proc": true, "sys": true, "dev": true,
	"run": true, "lost+found": true, "appdata": true,
}

func shouldExcludeSystemPath(relative string) bool {
	clean := filepath.Clean(relative)
	if clean == "." || clean == "" || clean == string(filepath.Separator) {
		return false
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	for _, part := range parts {
		name := strings.ToLower(part)
		if strings.HasPrefix(name, ".") || systemExcludedNames[name] {
			return true
		}
	}
	return false
}

// nextBackoff 依掃描時鐘估算下一個間隔變化的時間；實際仍在下一次掃描才套用。
func (s *Scanner) nextBackoff(now, scanActivity time.Time, tier string) (time.Time, time.Duration, bool) {
	age := now.Sub(scanActivity)
	switch tier {
	case "hot":
		if age < time.Hour {
			return scanActivity.Add(time.Hour), s.warm, true
		}
	case "warm":
		if age < 6*time.Hour {
			return scanActivity.Add(6 * time.Hour), s.warmSlow, true
		}
		if age < 24*time.Hour {
			return scanActivity.Add(24 * time.Hour), s.cold, true
		}
	case "cold":
		switch {
		case age < 48*time.Hour:
			return scanActivity.Add(48 * time.Hour), min(s.cold*3, s.idleMax), true
		case age < 72*time.Hour:
			return scanActivity.Add(72 * time.Hour), min(s.cold*6, s.idleMax), true
		case age < 120*time.Hour:
			return scanActivity.Add(120 * time.Hour), min(s.cold*12, s.idleMax), true
		case age < 168*time.Hour:
			return scanActivity.Add(168 * time.Hour), s.idleMax, true
		}
	}
	return time.Time{}, 0, false
}

// promotedScanActivity 回傳「往上一個掃描級別」的排程時鐘；不改變真實 lastActivity。
func (s *Scanner) promotedTier(tier string, interval time.Duration) (string, time.Duration) {
	switch tier {
	case "dormant":
		return "cold", min(s.cold*12, s.idleMax)
	case "cold":
		switch {
		case interval >= s.cold*9:
			return "cold", min(s.cold*6, s.idleMax)
		case interval >= s.cold*4:
			return "cold", min(s.cold*3, s.idleMax)
		case interval >= s.cold*2:
			return "cold", min(s.cold, s.idleMax)
		default:
			return "warm", s.warmSlow
		}
	case "warm":
		if interval >= s.warmSlow {
			return "warm", s.warm
		}
		return "hot", s.interval
	default:
		return "hot", s.interval
	}
}

func scanTier(now, lastActivity time.Time, base, warm, slowerWarm, cold, idleMax time.Duration) (string, time.Duration) {
	if base <= 0 {
		base = 30 * time.Second
	}
	if warm < base {
		warm = base
	}
	if slowerWarm < warm {
		slowerWarm = warm
	}
	if cold < slowerWarm {
		cold = slowerWarm
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
		age := now.Sub(lastActivity)
		interval := cold
		switch {
		case age < 48*time.Hour:
			interval = cold
		case age < 72*time.Hour:
			interval = cold * 3
		case age < 120*time.Hour:
			interval = cold * 6
		default:
			interval = cold * 12
		}
		if interval > dormant {
			interval = dormant
		}
		return "cold", interval
	}
	return "dormant", dormant
}

func (r *ScanResult) failure(err error) {
	r.Failed++
	if len(r.Errors) < 20 {
		r.Errors = append(r.Errors, err.Error())
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

// scanWebDAVPool 只為遠端 WebDAV 池建立 discovery；ETag 可辨識唯一 rename。
func (s *Scanner) scanWebDAVPool(ctx context.Context, pool Pool, now time.Time, result *ScanResult, activity map[string]scanActivity) {
	before := *result
	entries, err := s.store.listWebDAV(ctx, pool)
	if err != nil {
		result.failure(err)
		activity[pool.ID] = activityHot
		return
	}
	existing, err := s.store.discoveredIdentities(ctx, pool.ID)
	if err != nil {
		result.failure(err)
		activity[pool.ID] = activityHot
		return
	}
	for oldPath, newPath := range remoteRenameMap(existing, entries) {
		if err := s.store.renameRemotePath(ctx, pool.ID, oldPath, newPath); err != nil {
			result.failure(err)
			continue
		}
		result.Renamed++
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		key := fileKey(pool.ID, entry.Relative)
		seen[key] = true
		changed, err := s.store.upsertDiscovered(ctx, pool.ID, entry.Relative, entry.Size, entry.Modified, entry.ETag, now)
		if err != nil {
			result.failure(err)
			continue
		}
		if changed {
			result.Discovered++
		}
	}
	if err := s.store.markDiscoveredMissing(ctx, pool.ID, seen); err != nil {
		result.failure(err)
	}
	switch {
	case result.Discovered > before.Discovered || result.Failed > before.Failed || len(result.Errors) > len(before.Errors):
		activity[pool.ID] = activityHot
	case result.Renamed > before.Renamed:
		activity[pool.ID] = activityPromote
	}
}

func (s *Scanner) scanPools(ctx context.Context, now time.Time, pools []Pool, only map[string]bool) (ScanResult, map[string]scanActivity) {
	result := ScanResult{StartedAt: now, Errors: []string{}}
	activity := map[string]scanActivity{}
	catalog, importJobs, err := s.catalog(ctx)
	if err != nil {
		result.failure(err)
		return result, activity
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
		if pool.Provider == "webdav" {
			s.scanWebDAVPool(ctx, pool, now, &result, activity)
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
			if entry.Type()&os.ModeSymlink != 0 || isReparsePoint(path) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if pool.ScanExcludeSystem && isHiddenOrSystem(path) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				result.failure(err)
				return nil
			}
			if entry.IsDir() {
				if pool.ScanExcludeSystem && shouldExcludeSystemPath(relative) {
					return filepath.SkipDir
				}
				return nil
			}
			if pool.ScanExcludeSystem && shouldExcludeSystemPath(relative) {
				return nil
			}
			if !s.store.supportedLocalVideo(path) {
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
			if pool.IndexOnly {
				changed, err := s.store.upsertDiscovered(ctx, pool.ID, relative, info.Size(), info.ModTime(), "", now)
				if err != nil {
					result.failure(err)
				} else if changed {
					result.Discovered++
				}
				delete(next, key)
				return nil
			}
			if !pool.IndexOnly {
				discovered, err := s.store.hasDiscovered(ctx, pool.ID, relative)
				if err != nil {
					result.failure(err)
					return nil
				}
				if discovered {
					result.Skipped++
					return nil
				}
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
			if err := s.store.markDiscoveredMissing(ctx, pool.ID, seen); err != nil {
				result.failure(err)
			}
		}
		if complete {
			for key, video := range catalog {
				if video.poolID == pool.ID && !seen[key] {
					result.Deleted++
				}
			}
		}
		switch {
		case result.Imported > before.Imported || result.Enqueued > before.Enqueued || result.Pending > before.Pending ||
			result.Discovered > before.Discovered || result.Failed > before.Failed || len(result.Errors) > len(before.Errors):
			activity[pool.ID] = activityHot
		case result.Changed > before.Changed || result.Deleted > before.Deleted || result.Renamed > before.Renamed:
			activity[pool.ID] = activityPromote
		}
	}
	s.observed = next
	return result, activity
}
