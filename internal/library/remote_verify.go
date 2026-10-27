package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const verifyChunkSize = 16 << 20

var errVerifyCancelled = errors.New("verify cancelled")

// EnqueueVerify 建立或重用遠端 SHA-256 驗證工作。
func (q *ImportQueue) EnqueueVerify(ctx context.Context, videoID string) (ImportJob, error) {
	video, err := q.store.video(videoID)
	if err != nil {
		return ImportJob{}, err
	}
	if video.Provider != "webdav" {
		return ImportJob{}, fmt.Errorf("此影片不是遠端來源")
	}
	if video.HashStatus == "verified" {
		return ImportJob{}, fmt.Errorf("影片已驗證")
	}
	key := "verify:" + video.ID
	now := timestamp()
	_, err = q.store.db.ExecContext(ctx, `INSERT INTO import_jobs(id,pool_id,relative_path,path_key,title,metadata_json,source,status,phase,bytes_processed,total_bytes,video_id,error,attempts,created_at,updated_at)
VALUES(?,?,?,?,?,'{}','verify','queued','queued',0,?,?,'',0,?,?)
ON CONFLICT(pool_id,path_key) DO UPDATE SET title=excluded.title,status='queued',phase='queued',bytes_processed=0,error='',created_at=excluded.created_at,updated_at=excluded.updated_at
WHERE import_jobs.source='verify' AND import_jobs.status IN ('completed','failed','cancelled')`, identifier(), video.PoolID, video.relativePath, key, video.Title, video.Size, video.ID, now, now)
	if err != nil {
		return ImportJob{}, err
	}
	job, err := scanJob(q.store.db.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM import_jobs WHERE pool_id=? AND path_key=?", video.PoolID, key))
	if err != nil {
		return ImportJob{}, err
	}
	if _, err := q.store.db.ExecContext(ctx, "UPDATE videos SET hash_status='pending' WHERE id=?", video.ID); err != nil {
		return ImportJob{}, err
	}
	q.notify()
	q.store.publish("jobs")
	return job, nil
}

// CancelVerify 取消等待或執行中的驗證工作。
func (q *ImportQueue) CancelVerify(ctx context.Context, videoID string) (ImportJob, error) {
	video, err := q.store.video(videoID)
	if err != nil {
		return ImportJob{}, err
	}
	key := "verify:" + video.ID
	result, err := q.store.db.ExecContext(ctx, "UPDATE import_jobs SET status='cancelled',phase='cancelled',error='',updated_at=? WHERE pool_id=? AND path_key=? AND source='verify' AND status IN ('queued','running')", timestamp(), video.PoolID, key)
	if err != nil {
		return ImportJob{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ImportJob{}, err
	}
	if affected == 0 {
		return ImportJob{}, sql.ErrNoRows
	}
	if _, err := q.store.db.ExecContext(ctx, "UPDATE videos SET hash_status='unverified' WHERE id=? AND hash_status='pending'", video.ID); err != nil {
		return ImportJob{}, err
	}
	q.store.publish("jobs")
	return scanJob(q.store.db.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM import_jobs WHERE pool_id=? AND path_key=?", video.PoolID, key))
}

func (q *ImportQueue) processRemoteVerify(ctx context.Context, job ImportJob) error {
	video, err := q.store.video(job.VideoID)
	if err != nil {
		return err
	}
	pool, err := q.store.poolByID(video.PoolID)
	if err != nil || !pool.Enabled {
		return ErrPoolDisabled
	}
	username, password, err := q.store.WebDAVCredentials(pool.ID)
	if err != nil {
		return err
	}
	target, err := webDAVTarget(pool, video.relativePath)
	if err != nil {
		return err
	}
	size, etag, modified, err := webDAVHead(ctx, target, username, password)
	if err != nil {
		return err
	}
	if size > 0 && video.Size > 0 && size != video.Size {
		return fmt.Errorf("遠端檔案大小已變更")
	}
	total := video.Size
	if total <= 0 {
		total = size
	}
	if total <= 0 {
		return fmt.Errorf("遠端檔案大小未知")
	}
	failed := true
	defer func() {
		if failed {
			_ = q.store.setVideoHashStatus(video.ID, "failed")
		}
	}()
	hasher := sha256.New()
	var processed int64
	for processed < total {
		status, err := q.verifyJobStatus(ctx, job.ID)
		if err != nil {
			return err
		}
		if status == "cancelled" {
			failed = false
			_ = q.store.setVideoHashStatus(video.ID, "unverified")
			return errVerifyCancelled
		}
		end := processed + verifyChunkSize - 1
		if end >= total {
			end = total - 1
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return err
		}
		request.SetBasicAuth(username, password)
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", processed, end))
		if etag != "" {
			request.Header.Set("If-Range", etag)
		} else if modified != "" {
			request.Header.Set("If-Range", modified)
		}
		response, err := (&http.Client{}).Do(request)
		if err != nil {
			return err
		}
		if response.StatusCode == http.StatusOK {
			// 伺服器忽略 Range：只能在第一塊時退回完整串流。
			if processed != 0 {
				response.Body.Close()
				return fmt.Errorf("WebDAV 忽略 Range，無法續傳")
			}
			reader := io.Reader(response.Body)
			if q.store.hashLimitBytesPerSecond > 0 {
				reader = &throttledReader{reader: reader, bytesPerSecond: q.store.hashLimitBytesPerSecond, started: time.Now()}
			}
			n, copyErr := io.Copy(hasher, reader)
			response.Body.Close()
			if copyErr != nil {
				return copyErr
			}
			processed = n
			_ = q.updateVerifyProgress(job.ID, processed)
			break
		}
		if response.StatusCode != http.StatusPartialContent {
			response.Body.Close()
			return fmt.Errorf("WebDAV Range 回應 %s", response.Status)
		}
		start, gotEnd, gotTotal, ok := parseContentRange(response.Header.Get("Content-Range"))
		if !ok || start != processed || gotEnd != end || (gotTotal > 0 && gotTotal != total) {
			response.Body.Close()
			return fmt.Errorf("WebDAV Content-Range 不一致")
		}
		reader := io.Reader(response.Body)
		if q.store.hashLimitBytesPerSecond > 0 {
			reader = &throttledReader{reader: reader, bytesPerSecond: q.store.hashLimitBytesPerSecond, started: time.Now()}
		}
		length := end - processed + 1
		n, copyErr := io.CopyN(hasher, reader, length)
		response.Body.Close()
		if copyErr != nil {
			return copyErr
		}
		if n != length {
			return fmt.Errorf("WebDAV Range 長度不足")
		}
		processed += n
		if err := q.updateVerifyProgress(job.ID, processed); err != nil {
			return err
		}
	}
	if processed != total {
		return fmt.Errorf("驗證讀取長度不符：%d/%d", processed, total)
	}
	finalSize, finalEtag, finalModified, err := webDAVHead(ctx, target, username, password)
	if err != nil {
		return err
	}
	if finalSize != size || finalEtag != etag || finalModified != modified {
		return fmt.Errorf("遠端檔案在驗證期間已變更")
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	if _, err := q.store.db.Exec("UPDATE videos SET file_hash=?,source_hash=?,hash_status='verified' WHERE id=?", digest, digest, video.ID); err != nil {
		return err
	}
	if _, err := q.store.db.Exec("UPDATE import_jobs SET status='completed',phase='completed',bytes_processed=?,error='',updated_at=? WHERE id=? AND status='running'", total, timestamp(), job.ID); err != nil {
		return err
	}
	failed = false
	q.store.publish("jobs")
	return nil
}

func webDAVHead(ctx context.Context, target *url.URL, username, password string) (int64, string, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, target.String(), nil)
	if err != nil {
		return 0, "", "", err
	}
	request.SetBasicAuth(username, password)
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return 0, "", "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, "", "", fmt.Errorf("WebDAV HEAD %s", response.Status)
	}
	size, _ := strconv.ParseInt(response.Header.Get("Content-Length"), 10, 64)
	return size, response.Header.Get("ETag"), response.Header.Get("Last-Modified"), nil
}

func parseContentRange(value string) (int64, int64, int64, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "bytes ") {
		return 0, 0, 0, false
	}
	value = strings.TrimPrefix(value, "bytes ")
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0, 0, 0, false
	}
	rangePart := strings.Split(parts[0], "-")
	if len(rangePart) != 2 {
		return 0, 0, 0, false
	}
	start, err1 := strconv.ParseInt(rangePart[0], 10, 64)
	end, err2 := strconv.ParseInt(rangePart[1], 10, 64)
	total, err3 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || start < 0 || end < start {
		return 0, 0, 0, false
	}
	return start, end, total, true
}

func (q *ImportQueue) verifyJobStatus(ctx context.Context, jobID string) (string, error) {
	var status string
	err := q.store.db.QueryRowContext(ctx, "SELECT status FROM import_jobs WHERE id=?", jobID).Scan(&status)
	return status, err
}

func (q *ImportQueue) updateVerifyProgress(jobID string, processed int64) error {
	_, err := q.store.db.Exec("UPDATE import_jobs SET phase='hashing',bytes_processed=?,updated_at=? WHERE id=? AND status='running'", processed, timestamp(), jobID)
	if err == nil {
		q.store.publish("jobs")
	}
	return err
}

func (s *Store) setVideoHashStatus(videoID, status string) error {
	_, err := s.db.Exec("UPDATE videos SET hash_status=? WHERE id=?", status, videoID)
	if err == nil {
		s.publish("jobs")
	}
	return err
}

type throttledReader struct {
	reader         io.Reader
	bytesPerSecond int64
	started        time.Time
	read           int64
}

func (r *throttledReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += int64(n)
	if r.bytesPerSecond > 0 && r.read > 0 {
		expected := time.Duration(float64(r.read) / float64(r.bytesPerSecond) * float64(time.Second))
		if elapsed := time.Since(r.started); elapsed < expected {
			time.Sleep(expected - elapsed)
		}
	}
	return n, err
}
