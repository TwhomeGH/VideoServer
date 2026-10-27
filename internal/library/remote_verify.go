package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
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
	if _, err := io.Copy(hasher, response.Body); err != nil {
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

func (s *Store) setHashStatus(id, status string) {
	_, _ = s.db.Exec("UPDATE videos SET hash_status=? WHERE id=?", status, id)
	s.publish("jobs")
}
