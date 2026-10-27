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
	if _, err := q.store.poolPath(video.PoolID); err != nil {
		return ImportJob{}, err
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
	}
	return job, err
}

func (q *ImportQueue) processProbe(ctx context.Context, job ImportJob) error {
	video, err := q.store.video(job.VideoID)
	if err != nil {
		return err
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
	return tx.Commit()
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
	LastScheduledAt    string `json:"lastScheduledAt,omitempty"`
	LastVideoID        string `json:"lastVideoId,omitempty"`
}

// BackfillStatus 回傳目前設定、下次檢查時間與待檢查候選數。
func (q *ImportQueue) BackfillStatus(ctx context.Context) (BackfillStatus, error) {
	q.backfillMu.Lock()
	status := q.backfill
	q.backfillMu.Unlock()
	if status.NextCheckAt != "" {
		if next, err := time.Parse(time.RFC3339Nano, status.NextCheckAt); err == nil {
			if remaining := int(time.Until(next).Seconds()); remaining > 0 {
				status.NextCheckInSeconds = remaining
			}
		}
	}
	err := q.store.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM videos v
JOIN pools p ON p.id = v.pool_id AND p.enabled = 1
LEFT JOIN video_media m ON m.video_id = v.id
WHERE m.video_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM import_jobs j WHERE j.source = 'reprobe' AND j.video_id = v.id)
  AND NOT EXISTS (SELECT 1 FROM import_jobs j WHERE j.status IN ('queued','running'))`).Scan(&status.CandidateCount)
	return status, err
}

// RunBackfill 週期性補齊缺少技術資訊的影片；一次只排一筆，且只在整個佇列閒置時排。
func (q *ImportQueue) RunBackfill(ctx context.Context, interval time.Duration, enabled bool) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	q.backfillMu.Lock()
	q.backfill = BackfillStatus{Enabled: enabled, IntervalSeconds: int(interval / time.Second)}
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
		job, added, err := q.BackfillMissingMedia(ctx)
		now := time.Now().UTC()
		q.backfillMu.Lock()
		q.backfill.NextCheckAt = now.Add(interval).Format(time.RFC3339Nano)
		if added {
			q.backfill.LastScheduledAt = now.Format(time.RFC3339Nano)
			q.backfill.LastVideoID = job.VideoID
		}
		q.backfillMu.Unlock()
		if err != nil && ctx.Err() == nil {
			q.logger.Error("自動補齊影片技術資訊失敗", "error", err)
		}
		timer.Reset(interval)
	}
}
