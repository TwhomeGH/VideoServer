package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/media"
)

// ErrImportExists 表示檔案已索引或已有工作，避免新請求覆寫既有描述。
var ErrImportExists = errors.New("此檔案已索引或已有匯入工作；請查看工作清單、重試失敗工作，或編輯已匯入影片")

// ImportJob 是持久化的匯入或重新探測狀態；新匯入完成前不提供播放入口。
type ImportJob struct {
	ID             string `json:"id"`
	PoolID         string `json:"poolId"`
	Path           string `json:"path"`
	Title          string `json:"title"`
	Source         string `json:"source"`
	Status         string `json:"status"`
	Phase          string `json:"phase"`
	BytesProcessed int64  `json:"bytesProcessed"`
	TotalBytes     int64  `json:"totalBytes"`
	VideoID        string `json:"videoId"`
	Error          string `json:"error"`
	Attempts       int    `json:"attempts"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	metadata       Metadata
}

const jobColumns = `id,pool_id,relative_path,title,source,status,phase,bytes_processed,total_bytes,video_id,error,attempts,created_at,updated_at,metadata_json`

func scanJob(row interface{ Scan(...any) error }) (ImportJob, error) {
	var job ImportJob
	var metadata string
	err := row.Scan(&job.ID, &job.PoolID, &job.Path, &job.Title, &job.Source, &job.Status, &job.Phase, &job.BytesProcessed, &job.TotalBytes, &job.VideoID, &job.Error, &job.Attempts, &job.CreatedAt, &job.UpdatedAt, &metadata)
	if err == nil {
		err = json.Unmarshal([]byte(metadata), &job.metadata)
	}
	return job, err
}

// ImportQueue 將手動匯入與自動發現串成單一持久背景佇列。
type ImportQueue struct {
	store           *Store
	wake            chan struct{}
	logger          *slog.Logger
	probe           func(context.Context, string) (media.Info, error)
	backfillSkipped map[string]struct{}
	backfillMu      sync.Mutex
	backfill        BackfillStatus
}

// NewImportQueue 建立 store 的背景匯入器，並將上次中斷的工作還原成等待。
// ffprobePath 是本機執行檔，timeout 限制單次檢查；資料庫不可共用於多個節點程序。
func NewImportQueue(store *Store, ffprobePath string, timeout time.Duration, logger *slog.Logger) (*ImportQueue, error) {
	if strings.TrimSpace(ffprobePath) == "" || timeout <= 0 {
		return nil, fmt.Errorf("FFprobe 路徑與逾時設定無效")
	}
	if _, err := store.db.Exec(`UPDATE import_jobs SET status='queued',phase='queued',bytes_processed=0,error='',updated_at=? WHERE status='running'`, timestamp()); err != nil {
		return nil, err
	}
	queue := &ImportQueue{store: store, wake: make(chan struct{}, 1), logger: logger, backfillSkipped: map[string]struct{}{}, probe: func(ctx context.Context, path string) (media.Info, error) {
		return media.Probe(ctx, path, ffprobePath, timeout)
	}}
	store.queue = queue
	return queue, nil
}

// SetHashLimit 設定背景 SHA-256 讀取上限；0 表示不限速。
func (q *ImportQueue) SetHashLimit(mibPerSecond int) {
	if mibPerSecond < 0 {
		mibPerSecond = 0
	}
	q.store.hashLimitBytesPerSecond = int64(mibPerSecond) * 1024 * 1024
}

func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func (q *ImportQueue) notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Enqueue 驗證路徑及描述後加入工作，不在 HTTP 請求內計算雜湊或執行 FFprobe。
// source 為 manual 或 scan；scan 遇到既有工作回傳 added=false，manual 回傳 ErrImportExists。
func (q *ImportQueue) Enqueue(ctx context.Context, poolID, relative, title, source string, metadata Metadata) (ImportJob, bool, error) {
	var empty ImportJob
	if source != "manual" && source != "scan" {
		return empty, false, fmt.Errorf("未知匯入來源")
	}
	details, err := metadata.normalize()
	if err != nil {
		return empty, false, err
	}
	root, err := q.store.poolPath(poolID)
	if err != nil {
		return empty, false, err
	}
	path, err := resolve(root, relative)
	if err != nil {
		return empty, false, err
	}
	if !supportedVideo(path) {
		return empty, false, fmt.Errorf("不支援的影片副檔名")
	}
	info, err := os.Stat(path)
	if err != nil {
		return empty, false, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return empty, false, fmt.Errorf("來源必須是非空的一般影片檔案")
	}
	relative, err = filepath.Rel(root, path)
	if err != nil {
		return empty, false, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = filepath.Base(path)
	}
	key := fileKey(poolID, relative)
	var count int
	if err := q.store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM videos WHERE pool_id=? AND relative_path=?", poolID, relative).Scan(&count); err != nil {
		return empty, false, err
	}
	if count > 0 {
		return empty, false, ErrImportExists
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return empty, false, err
	}
	job := ImportJob{ID: identifier(), PoolID: poolID, Path: relative, Title: title, Source: source, Status: "queued", Phase: "queued", TotalBytes: info.Size(), CreatedAt: timestamp(), UpdatedAt: timestamp(), metadata: details}
	result, err := q.store.db.ExecContext(ctx, `INSERT INTO import_jobs(id,pool_id,relative_path,path_key,title,metadata_json,source,status,phase,bytes_processed,total_bytes,video_id,error,attempts,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,'queued','queued',0,?,'','',0,?,?) ON CONFLICT(pool_id,path_key) DO NOTHING`, job.ID, poolID, relative, key, title, string(payload), source, info.Size(), job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return empty, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return empty, false, err
	}
	if affected == 0 {
		if source == "manual" {
			return empty, false, ErrImportExists
		}
		existing, err := scanJob(q.store.db.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM import_jobs WHERE pool_id=? AND path_key=?", poolID, key))
		return existing, false, err
	}
	q.notify()
	return job, true, nil
}

// Jobs 回傳最近一百筆工作；工作本身保留於 SQLite，不因重啟遺失。
func (q *ImportQueue) Jobs() ([]ImportJob, error) {
	rows, err := q.store.db.Query("SELECT " + jobColumns + " FROM import_jobs ORDER BY created_at DESC,id LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ImportJob{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

// Job 取得 id 的工作，不存在時回傳 sql.ErrNoRows。
func (q *ImportQueue) Job(id string) (ImportJob, error) {
	return scanJob(q.store.db.QueryRow("SELECT "+jobColumns+" FROM import_jobs WHERE id=?", id))
}

// Retry 只將失敗工作重新排程，保留原描述與工作識別，不覆寫已完成影片。
func (q *ImportQueue) Retry(ctx context.Context, id string) (ImportJob, error) {
	result, err := q.store.db.ExecContext(ctx, `UPDATE import_jobs SET status='queued',phase='queued',bytes_processed=0,error='',updated_at=? WHERE id=? AND status='failed'`, timestamp(), id)
	if err != nil {
		return ImportJob{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ImportJob{}, err
	}
	if affected == 0 {
		return ImportJob{}, fmt.Errorf("只有失敗工作可以重試")
	}
	q.notify()
	return q.Job(id)
}

// Run 處理佇列直到 ctx 取消；同一佇列只能執行一個 worker。
// 取消中的工作回到 queued，非正常中止則由下次 NewImportQueue 恢復。
func (q *ImportQueue) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		job, err := scanJob(q.store.db.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM import_jobs WHERE status='queued' ORDER BY created_at,id LIMIT 1"))
		if err == sql.ErrNoRows {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-q.wake:
			}
			continue
		}
		if err != nil {
			q.logger.Error("讀取匯入工作失敗", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		claimed, err := q.store.db.ExecContext(ctx, `UPDATE import_jobs SET status='running',phase='probing',attempts=attempts+1,updated_at=? WHERE id=? AND status='queued'`, timestamp(), job.ID)
		if err != nil {
			q.logger.Error("領取匯入工作失敗", "jobId", job.ID, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		count, err := claimed.RowsAffected()
		if err != nil || count != 1 {
			continue
		}
		err = q.process(ctx, job)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			recovery, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, recoveryErr := q.store.db.ExecContext(recovery, `UPDATE import_jobs SET status='queued',phase='queued',bytes_processed=0,error='',updated_at=? WHERE id=? AND status='running'`, timestamp(), job.ID)
			cancel()
			if recoveryErr != nil {
				q.logger.Error("保存中斷匯入工作失敗", "jobId", job.ID, "error", recoveryErr)
			}
			return
		}
		if _, saveErr := q.store.db.ExecContext(ctx, `UPDATE import_jobs SET status='failed',phase='failed',error=?,updated_at=? WHERE id=? AND status='running'`, err.Error(), timestamp(), job.ID); saveErr != nil {
			q.logger.Error("保存失敗工作狀態失敗", "jobId", job.ID, "error", saveErr)
		}
		q.logger.Warn("影片背景工作失敗", "jobId", job.ID, "source", job.Source, "error", err)
	}
}

func (q *ImportQueue) process(ctx context.Context, job ImportJob) error {
	if job.Source == "reprobe" {
		return q.processProbe(ctx, job)
	}
	root, err := q.store.poolPath(job.PoolID)
	if err != nil {
		return err
	}
	path, err := resolve(root, job.Path)
	if err != nil {
		return err
	}
	before, err := os.Stat(path)
	if err != nil {
		return err
	}
	info, err := q.probe(ctx, path)
	if err != nil {
		return err
	}
	after, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return fmt.Errorf("影片檢查期間來源已變更，請待寫入完成後重試")
	}
	if _, err := q.store.db.ExecContext(ctx, `UPDATE import_jobs SET phase='hashing',total_bytes=?,updated_at=? WHERE id=?`, before.Size(), timestamp(), job.ID); err != nil {
		return err
	}
	_, err = q.store.importVideo(ctx, job.PoolID, job.Path, job.Title, job.metadata, &importOptions{media: &info, jobID: job.ID, expected: before})
	return err
}
