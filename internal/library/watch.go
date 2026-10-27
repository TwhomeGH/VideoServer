package library

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watchRefreshInterval = 10 * time.Second
const watchDebounce = 500 * time.Millisecond

// WatchStatus 提供監聽器目前的可觀察狀態；實際檔案變更仍以 reconciliation 掃描為準。
type WatchStatus struct {
	Enabled         bool     `json:"enabled"`
	Mode            string   `json:"mode"`
	ActivePools     int      `json:"activePools"`
	Events          uint64   `json:"events"`
	LastEventAt     string   `json:"lastEventAt,omitempty"`
	FallbackReasons []string `json:"fallbackReasons,omitempty"`
}

type poolWatch struct {
	cancel  context.CancelFunc
	watcher *fsnotify.Watcher
}

// Watcher 只做新檔事件的低延遲提示，不直接修改 catalog 或匯入。
type Watcher struct {
	store    *Store
	scanner  *Scanner
	logger   *slog.Logger
	mu       sync.Mutex
	pools    map[string]*poolWatch
	fallback map[string]string
	events   uint64
	last     time.Time
}

func NewWatcher(store *Store, scanner *Scanner, logger *slog.Logger) *Watcher {
	return &Watcher{store: store, scanner: scanner, logger: logger, pools: map[string]*poolWatch{}, fallback: map[string]string{}}
}

func (w *Watcher) Run(ctx context.Context) {
	w.syncPools(ctx)
	ticker := time.NewTicker(watchRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.stopAll()
			return
		case <-ticker.C:
			w.syncPools(ctx)
		}
	}
}

func (w *Watcher) Status() WatchStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	reasons := make([]string, 0, len(w.fallback))
	seen := map[string]bool{}
	for _, reason := range w.fallback {
		if !seen[reason] {
			seen[reason] = true
			reasons = append(reasons, reason)
		}
	}
	sort.Strings(reasons)
	status := WatchStatus{Enabled: true, Mode: "auto", ActivePools: len(w.pools), Events: w.events, FallbackReasons: reasons}
	if !w.last.IsZero() {
		status.LastEventAt = w.last.UTC().Format(time.RFC3339Nano)
	}
	return status
}

func (w *Watcher) syncPools(ctx context.Context) {
	pools, err := w.store.Pools()
	if err != nil {
		w.logger.Warn("檔案監聽無法讀取儲存池", "error", err)
		return
	}
	desired := map[string]Pool{}
	for _, pool := range pools {
		if !pool.Enabled || !pool.Online {
			continue
		}
		if pool.WholeDisk {
			w.setFallback(pool.ID, "整碟池使用輪詢")
			continue
		}
		if strings.Contains(strings.ToLower(pool.Filesystem), "9p") {
			w.setFallback(pool.ID, "9p 檔案系統使用輪詢")
			continue
		}
		desired[pool.ID] = pool
	}
	w.mu.Lock()
	for id, watch := range w.pools {
		if _, exists := desired[id]; !exists {
			watch.cancel()
			watch.watcher.Close()
			delete(w.pools, id)
			delete(w.fallback, id)
		}
	}
	w.mu.Unlock()
	for id, pool := range desired {
		w.mu.Lock()
		_, exists := w.pools[id]
		w.mu.Unlock()
		if !exists {
			w.startPool(ctx, pool)
		}
	}
}

func (w *Watcher) startPool(ctx context.Context, pool Pool) {
	root, err := w.store.poolPath(pool.ID)
	if err != nil {
		w.setFallback(pool.ID, "儲存池路徑無法使用")
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		w.setFallback(pool.ID, "無法啟動檔案監聽："+err.Error())
		return
	}
	added, err := addWatchTree(watcher, root, pool)
	if err != nil || added == 0 {
		watcher.Close()
		reason := "沒有可監聽的目錄"
		if err != nil {
			reason = err.Error()
		}
		w.setFallback(pool.ID, reason)
		return
	}
	poolCtx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.pools[pool.ID] = &poolWatch{cancel: cancel, watcher: watcher}
	delete(w.fallback, pool.ID)
	w.mu.Unlock()
	go w.runPool(poolCtx, pool, watcher)
	w.logger.Info("檔案監聽已啟動", "poolId", pool.ID, "directories", added)
}

func (w *Watcher) runPool(ctx context.Context, pool Pool, watcher *fsnotify.Watcher) {
	var timer *time.Timer
	notify := func() {
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(watchDebounce, func() {
			w.scanner.NotifyChange(pool.ID)
		})
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
		watcher.Close()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_, _ = addWatchTree(watcher, event.Name, pool)
				}
			}
			w.mu.Lock()
			w.events++
			w.last = time.Now()
			w.mu.Unlock()
			notify()
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			if !errors.Is(err, context.Canceled) {
				w.logger.Warn("檔案監聽錯誤", "poolId", pool.ID, "error", err)
				w.setFallback(pool.ID, "檔案監聽錯誤；輪詢仍會運作")
			}
		}
	}
}

func (w *Watcher) setFallback(poolID, reason string) {
	w.mu.Lock()
	w.fallback[poolID] = reason
	w.mu.Unlock()
}

func (w *Watcher) stopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, watch := range w.pools {
		watch.cancel()
		watch.watcher.Close()
		delete(w.pools, id)
	}
}

func addWatchTree(watcher *fsnotify.Watcher, root string, pool Pool) (int, error) {
	added := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || isReparsePoint(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if pool.ScanExcludeSystem && shouldExcludeSystemPath(relative) {
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			return nil
		}
		added++
		return nil
	})
	return added, err
}
