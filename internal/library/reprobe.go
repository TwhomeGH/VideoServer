package library

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// EnqueueProbe 為 videoID 重新讀取技術資訊，不變更影片識別、描述或檔案。
// 每部影片沿用一筆 reprobe 工作；等待或執行中的重複請求回傳同一工作。
// 影片不存在時回傳 sql.ErrNoRows，離線與來源變更由背景工作回報。
func (q *ImportQueue) EnqueueProbe(ctx context.Context, videoID string) (ImportJob, error) {
	video, err := q.store.video(videoID)
	if err != nil {
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
