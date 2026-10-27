package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// EnqueueProbe 為 videoID 重新讀取技術資訊，不變更影片識別、描述或檔案。
// 每部影片沿用一筆 reprobe 工作；等待或執行中的重複請求回傳同一工作。
// 影片不存在時回傳 sql.ErrNoRows，離線與來源變更由背景工作回報。
func (q *ImportQueue) EnqueueProbe(ctx context.Context, videoID string) (ImportJob, error) {
	video, err := q.store.video(videoID)
	if err != nil {
		return ImportJob{}, err
	}
	pool, err := q.store.poolByID(video.PoolID)
	if err != nil {
		return ImportJob{}, err
	}
	if pool.Provider != "webdav" {
		if _, err := q.store.poolPath(video.PoolID); err != nil {
			return ImportJob{}, err
		}
	}
	key := "reprobe:" + video.ID
	now := timestamp()
	_, err = q.store.db.ExecContext(ctx, `INSERT INTO import_jobs(id,pool_id,relative_path,path_key,title,metadata_json,source,status,phase,bytes_processed,total_bytes,video_id,error,attempts,created_at,updated_at)
VALUES(?,?,?,?,?,'{}','reprobe','queued','queued',0,?,?,'',0,?,?)
ON CONFLICT(pool_id,path_key) DO UPDATE SET title=excluded.title,status='queued',phase='queued',bytes_processed=0,error='',created_at=excluded.created_at,updated_at=excluded.updated_at
WHERE import_jobs.source='reprobe' AND import_jobs.status IN ('completed','failed')`, identifier(), video.PoolID, video.relativePath, key, video.Title, video.Size, video.ID, now, now)
	if err != nil {
		return ImportJob{}, err
	}
	job, err := scanJob(q.store.db.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM import_jobs WHERE pool_id=? AND path_key=?", video.PoolID, key))
	if err == nil {
		q.notify()
		q.store.publish("jobs")
	}
	return job, err
}

func (q *ImportQueue) processProbe(ctx context.Context, job ImportJob) error {
	video, err := q.store.video(job.VideoID)
	if err != nil {
		return err
	}
	if video.Provider == "webdav" {
		return q.processRemoteProbe(ctx, job, video)
	}
	file, err := q.store.openVideo(video)
	if err != nil {
		return err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return err
	}
	root, err := q.store.poolPath(video.PoolID)
	if err != nil {
		return err
	}
	path, err := resolve(root, video.relativePath)
	if err != nil {
		return err
	}
	info, err := q.probe(ctx, path)
	if err != nil {
		return err
	}
	if _, err := q.store.db.ExecContext(ctx, "UPDATE import_jobs SET phase='hashing',updated_at=? WHERE id=? AND status='running'", timestamp(), job.ID); err != nil {
		return err
	}
	q.store.publish("jobs")
	digest, err := q.store.hashSource(ctx, file, video.Size, job.ID)
	if err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(before, current) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || current.Size() != before.Size() || !current.ModTime().Equal(before.ModTime()) || digest != video.FileHash {
		return fmt.Errorf("來源已變更，無法更新原影片技術資訊；請確認原始檔案")
	}
	payload, err := json.Marshal(info)
	if err != nil {
		return err
	}
	tx, err := q.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 只寫入技術資訊，不使用工作建立時的描述快照，避免蓋掉處理期間的編輯。
	if _, err := tx.ExecContext(ctx, "INSERT INTO video_media VALUES(?,?) ON CONFLICT(video_id) DO UPDATE SET details_json=excluded.details_json", video.ID, string(payload)); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status='completed',phase='completed',bytes_processed=?,error='',updated_at=? WHERE id=? AND status='running'`, video.Size, timestamp(), job.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("影片資訊工作狀態已改變")
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	q.store.publish("jobs")
	return nil
}

type missingMediaCandidate struct {
	id       string
	poolID   string
	relative string
	size     int64
	modified int64
}

// BackfillMissingMedia 只排入「沒有 video_media、也完全沒有 reprobe 工作記錄」的影片。
// 一次最多排一筆；離線、大小/修改時間不符或曾失敗的影片不自動重試，避免無限失敗。
func (q *ImportQueue) BackfillMissingMedia(ctx context.Context) (ImportJob, bool, error) {
	rows, err := q.store.db.QueryContext(ctx, `
SELECT v.id, v.pool_id, v.relative_path, v.size, v.modified_at
FROM videos v
JOIN pools p ON p.id = v.pool_id AND p.enabled = 1
LEFT JOIN video_media m ON m.video_id = v.id
WHERE m.video_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM import_jobs j WHERE j.source = 'reprobe' AND j.video_id = v.id)
  AND NOT EXISTS (SELECT 1 FROM import_jobs j WHERE j.status IN ('queued','running'))
ORDER BY v.imported_at, v.id`)
	if err != nil {
		return ImportJob{}, false, err
	}
	candidates := []missingMediaCandidate{}
	for rows.Next() {
		var candidate missingMediaCandidate
		if err := rows.Scan(&candidate.id, &candidate.poolID, &candidate.relative, &candidate.size, &candidate.modified); err != nil {
			rows.Close()
			return ImportJob{}, false, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ImportJob{}, false, err
	}
	rows.Close()

	skipped := 0
	for _, candidate := range candidates {
		if _, seen := q.backfillSkipped[candidate.id]; seen {
			continue
		}
		root, err := q.store.poolPath(candidate.poolID)
		if err != nil {
			q.backfillSkipped[candidate.id] = struct{}{}
			skipped++
			continue
		}
		path, err := resolve(root, candidate.relative)
		if err != nil {
			q.backfillSkipped[candidate.id] = struct{}{}
			skipped++
			continue
		}
		info, statErr := os.Stat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != candidate.size || info.ModTime().UnixNano() != candidate.modified {
			q.backfillSkipped[candidate.id] = struct{}{}
			skipped++
			continue
		}
		job, err := q.EnqueueProbe(ctx, candidate.id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				q.backfillSkipped[candidate.id] = struct{}{}
				skipped++
				continue
			}
			return ImportJob{}, false, err
		}
		q.logger.Info("已排程缺少技術資訊的影片", "videoId", candidate.id)
		return job, true, nil
	}
	if skipped > 0 {
		q.logger.Warn("自動補齊略過離線或已變更的影片", "count", skipped)
	}
	return ImportJob{}, false, nil
}

// BackfillStatus 是自動補齊的排程與候選摘要。
type BackfillStatus struct {
	Enabled            bool   `json:"enabled"`
	IntervalSeconds    int    `json:"intervalSeconds"`
	NextCheckAt        string `json:"nextCheckAt,omitempty"`
	NextCheckInSeconds int    `json:"nextCheckInSeconds"`
	CandidateCount     int    `json:"candidateCount"`
	NextVideoID        string `json:"nextVideoId,omitempty"`
	NextVideoTitle     string `json:"nextVideoTitle,omitempty"`
	DailyVideoLimit    int    `json:"dailyVideoLimit"`
	DailyVideos        int    `json:"dailyVideos"`
	DailyGiBLimit      int    `json:"dailyGiBLimit"`
	DailyBytes         int64  `json:"dailyBytes"`
	DailyResetAt       string `json:"dailyResetAt,omitempty"`
	LimitReached       bool   `json:"limitReached"`
	QueueIdle          bool   `json:"queueIdle"`
	LastScheduledAt    string `json:"lastScheduledAt,omitempty"`
	LastVideoID        string `json:"lastVideoId,omitempty"`
}

// SetBackfillLimits 設定自動補齊每日部數與容量上限；0 表示不限。
func (q *ImportQueue) SetBackfillLimits(videoLimit, giBLimit int) {
	if videoLimit < 0 {
		videoLimit = 0
	}
	if giBLimit < 0 {
		giBLimit = 0
	}
	q.backfillMu.Lock()
	q.backfillDailyLimit = videoLimit
	q.backfillDailyGiB = giBLimit
	q.backfill.DailyVideoLimit = videoLimit
	q.backfill.DailyGiBLimit = giBLimit
	q.backfillMu.Unlock()
}

func (q *ImportQueue) backfillLimits() (int, int) {
	q.backfillMu.Lock()
	defer q.backfillMu.Unlock()
	return q.backfillDailyLimit, q.backfillDailyGiB
}

// backfillUsage 回傳今日已排程的自動補齊用量與下一個本地日界。
func (q *ImportQueue) backfillUsage(ctx context.Context) (time.Time, int, int64, error) {
	now := time.Now()
	day := now.Format("2006-01-02")
	reset := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	var videos int
	var bytes int64
	err := q.store.db.QueryRowContext(ctx, "SELECT videos,bytes FROM backfill_usage WHERE day=?", day).Scan(&videos, &bytes)
	if errors.Is(err, sql.ErrNoRows) {
		return reset, 0, 0, nil
	}
	return reset, videos, bytes, err
}

func (q *ImportQueue) recordBackfillUsage(ctx context.Context, bytes int64) error {
	day := time.Now().Format("2006-01-02")
	_, err := q.store.db.ExecContext(ctx, `
INSERT INTO backfill_usage(day,videos,bytes) VALUES(?,1,?)
ON CONFLICT(day) DO UPDATE SET videos=videos+1, bytes=bytes+excluded.bytes`, day, bytes)
	return err
}

// ResetBackfillUsage 清空今天的自動補齊用量；不影響其他日期或既有工作。
func (q *ImportQueue) ResetBackfillUsage(ctx context.Context) (BackfillStatus, error) {
	day := time.Now().Format("2006-01-02")
	if _, err := q.store.db.ExecContext(ctx, "DELETE FROM backfill_usage WHERE day=?", day); err != nil {
		return BackfillStatus{}, err
	}
	q.logger.Warn("自動補齊今日用量已手動重設", "day", day)
	q.store.publish("jobs")
	return q.BackfillStatus(ctx)
}

// BackfillStatus 回傳目前設定、下次檢查時間與待檢查候選數。
func (q *ImportQueue) BackfillStatus(ctx context.Context) (BackfillStatus, error) {
	q.backfillMu.Lock()
	status := q.backfill
	status.DailyVideoLimit = q.backfillDailyLimit
	status.DailyGiBLimit = q.backfillDailyGiB
	q.backfillMu.Unlock()
	if status.NextCheckAt != "" {
		if next, err := time.Parse(time.RFC3339Nano, status.NextCheckAt); err == nil {
			if remaining := int(time.Until(next).Seconds()); remaining > 0 {
				status.NextCheckInSeconds = remaining
			}
		}
	}
	reset, videos, bytes, err := q.backfillUsage(ctx)
	if err != nil {
		return status, err
	}
	status.DailyVideos = videos
	status.DailyBytes = bytes
	status.DailyResetAt = reset.UTC().Format(time.RFC3339Nano)
	status.LimitReached = backfillLimitReached(videos, bytes, status.DailyVideoLimit, status.DailyGiBLimit)
	err = q.store.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM videos v
JOIN pools p ON p.id = v.pool_id AND p.enabled = 1
LEFT JOIN video_media m ON m.video_id = v.id
WHERE m.video_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM import_jobs j WHERE j.source = 'reprobe' AND j.video_id = v.id)`).Scan(&status.CandidateCount)
	if err != nil {
		return status, err
	}
	var busy int
	if err := q.store.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM import_jobs WHERE status IN ('queued','running'))").Scan(&busy); err != nil {
		return status, err
	}
	status.QueueIdle = busy == 0
	err = q.store.db.QueryRowContext(ctx, `
SELECT v.id, v.title
FROM videos v
JOIN pools p ON p.id = v.pool_id AND p.enabled = 1
LEFT JOIN video_media m ON m.video_id = v.id
WHERE m.video_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM import_jobs j WHERE j.source = 'reprobe' AND j.video_id = v.id)
ORDER BY v.imported_at, v.id
LIMIT 1`).Scan(&status.NextVideoID, &status.NextVideoTitle)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return status, err
	}
	return status, nil
}

// RunBackfill 週期性補齊缺少技術資訊的影片；一次只排一筆，且只在整個佇列閒置時排。
func (q *ImportQueue) RunBackfill(ctx context.Context, interval time.Duration, enabled bool) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	q.backfillMu.Lock()
	q.backfill = BackfillStatus{
		Enabled: enabled, IntervalSeconds: int(interval / time.Second),
		DailyVideoLimit: q.backfillDailyLimit, DailyGiBLimit: q.backfillDailyGiB,
	}
	q.backfillMu.Unlock()
	if !enabled {
		return
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		reset, videos, bytes, usageErr := q.backfillUsage(ctx)
		if usageErr != nil && ctx.Err() == nil {
			q.logger.Error("讀取自動補齊每日用量失敗", "error", usageErr)
		}
		videoLimit, giBLimit := q.backfillLimits()
		if usageErr == nil && backfillLimitReached(videos, bytes, videoLimit, giBLimit) {
			now := time.Now().UTC()
			q.backfillMu.Lock()
			q.backfill.NextCheckAt = now.Add(interval).Format(time.RFC3339Nano)
			q.backfill.DailyVideos = videos
			q.backfill.DailyBytes = bytes
			q.backfill.DailyResetAt = reset.UTC().Format(time.RFC3339Nano)
			q.backfill.LimitReached = true
			q.backfillMu.Unlock()
			timer.Reset(interval)
			continue
		}
		job, added, err := q.BackfillMissingMedia(ctx)
		now := time.Now().UTC()
		q.backfillMu.Lock()
		q.backfill.NextCheckAt = now.Add(interval).Format(time.RFC3339Nano)
		if added {
			q.backfill.LastScheduledAt = now.Format(time.RFC3339Nano)
			q.backfill.LastVideoID = job.VideoID
		}
		q.backfill.DailyVideos = videos
		q.backfill.DailyBytes = bytes
		q.backfill.DailyResetAt = reset.UTC().Format(time.RFC3339Nano)
		q.backfill.LimitReached = false
		q.backfillMu.Unlock()
		if added {
			if err := q.recordBackfillUsage(ctx, job.TotalBytes); err != nil && ctx.Err() == nil {
				q.logger.Error("記錄自動補齊每日用量失敗", "error", err)
			}
		}
		if err != nil && ctx.Err() == nil {
			q.logger.Error("自動補齊影片技術資訊失敗", "error", err)
		}
		timer.Reset(interval)
	}
}

func backfillLimitReached(videos int, bytes int64, videoLimit, giBLimit int) bool {
	if videoLimit > 0 && videos >= videoLimit {
		return true
	}
	if giBLimit > 0 && bytes >= int64(giBLimit)*1024*1024*1024 {
		return true
	}
	return false
}
