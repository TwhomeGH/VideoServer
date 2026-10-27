package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"time"
)

// hashSource 從 file 目前位置計算 SHA-256；jobID 非空時節流保存背景進度。
// ctx 取消時停止讀取；來源一致性由呼叫端在讀取前後檢查。
func (s *Store) hashSource(ctx context.Context, file *os.File, size int64, jobID string) (string, error) {
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	var processed int64
	lastProgress := time.Time{}
	lastEvent := time.Time{}
	started := time.Now()
	limit := s.hashLimitBytesPerSecond
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := file.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
			processed += int64(n)
			if jobID != "" && (time.Since(lastProgress) > 250*time.Millisecond || processed == size) {
				if _, err := s.db.ExecContext(ctx, "UPDATE import_jobs SET bytes_processed=?,updated_at=? WHERE id=? AND status='running'", processed, timestamp(), jobID); err != nil {
					return "", err
				}
				lastProgress = time.Now()
				if time.Since(lastEvent) >= time.Second || processed == size {
					s.publish("jobs")
					lastEvent = time.Now()
				}
			}
			if limit > 0 {
				expected := time.Duration(float64(processed) / float64(limit) * float64(time.Second))
				if wait := expected - time.Since(started); wait > 0 {
					timer := time.NewTimer(wait)
					select {
					case <-ctx.Done():
						timer.Stop()
						return "", ctx.Err()
					case <-timer.C:
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
