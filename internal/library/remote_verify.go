package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// VerifyRemoteVideo 啟動背景 SHA-256 驗證；不保存下載內容。
func (s *Store) VerifyRemoteVideo(ctx context.Context, id string) (Video, error) {
	video, err := s.video(id)
	if err != nil {
		return video, err
	}
	if video.Provider != "webdav" {
		return video, fmt.Errorf("此影片不是遠端來源")
	}
	if video.HashStatus == "verified" {
		return video, nil
	}
	if video.HashStatus == "pending" {
		return video, fmt.Errorf("驗證已在進行中")
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE videos SET hash_status='pending' WHERE id=?", id); err != nil {
		return video, err
	}
	video.HashStatus = "pending"
	s.publish("jobs")
	go s.verifyRemoteVideo(id)
	return video, nil
}

func (s *Store) verifyRemoteVideo(id string) {
	video, err := s.video(id)
	if err != nil {
		return
	}
	pool, err := s.poolByID(video.PoolID)
	if err != nil || !pool.Enabled {
		s.setHashStatus(id, "failed")
		return
	}
	username, password, err := s.WebDAVCredentials(pool.ID)
	if err != nil {
		s.setHashStatus(id, "failed")
		return
	}
	target, err := webDAVTarget(pool, video.relativePath)
	if err != nil {
		s.setHashStatus(id, "failed")
		return
	}
	request, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		s.setHashStatus(id, "failed")
		return
	}
	request.SetBasicAuth(username, password)
	response, err := (&http.Client{}).Do(request)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		if response != nil {
			response.Body.Close()
		}
		s.setHashStatus(id, "failed")
		return
	}
	defer response.Body.Close()
	hasher := sha256.New()
	reader := io.Reader(response.Body)
	if s.hashLimitBytesPerSecond > 0 {
		reader = &throttledReader{reader: reader, bytesPerSecond: s.hashLimitBytesPerSecond, started: time.Now()}
	}
	if _, err := io.Copy(hasher, reader); err != nil {
		s.setHashStatus(id, "failed")
		return
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	if _, err := s.db.Exec("UPDATE videos SET file_hash=?,source_hash=?,hash_status='verified' WHERE id=?", digest, digest, id); err != nil {
		log.Printf("更新遠端影片 hash 失敗：%v", err)
		s.setHashStatus(id, "failed")
		return
	}
	s.publish("jobs")
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

func (s *Store) setHashStatus(id, status string) {
	_, _ = s.db.Exec("UPDATE videos SET hash_status=? WHERE id=?", status, id)
	s.publish("jobs")
}
