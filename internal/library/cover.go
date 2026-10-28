package library

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/media"
)

const maxCoverBytes = 8 << 20

// GenerateCover 使用 FFmpeg 從目前影片來源擷取 JPEG 封面。
func (s *Store) GenerateCover(ctx context.Context, videoID string) (Video, error) {
	video, err := s.video(videoID)
	if err != nil {
		return Video{}, err
	}
	target := ""
	headers := ""
	caFile := ""
	cleanupCA := func() {}
	tlsSkipVerify := false
	if video.Provider == "webdav" {
		pool, err := s.poolByID(video.PoolID)
		if err != nil || !pool.Enabled {
			return Video{}, ErrPoolDisabled
		}
		username, password, err := s.WebDAVCredentials(pool.ID)
		if err != nil {
			return Video{}, err
		}
		remote, err := webDAVTarget(pool, video.relativePath)
		if err != nil {
			return Video{}, err
		}
		target = remote.String()
		headers = "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password)) + "\r\n"
		tlsSkipVerify = pool.TLSSkipVerify
		caFile, cleanupCA, err = s.webDAVCATempFile(pool)
		if err != nil {
			return Video{}, err
		}
	} else {
		file, err := s.openVideo(video)
		if err != nil {
			return Video{}, err
		}
		target = file.Name()
		file.Close()
	}
	defer cleanupCA()
	if err := os.MkdirAll(s.coverDirectory(), 0755); err != nil {
		return Video{}, err
	}
	pending, err := os.CreateTemp(s.coverDirectory(), ".cover-*.jpg")
	if err != nil {
		return Video{}, err
	}
	pendingPath := pending.Name()
	pending.Close()
	defer os.Remove(pendingPath)
	offset := 5 * time.Second
	if err := media.Thumbnail(ctx, target, headers, caFile, tlsSkipVerify, s.ffmpegPath, pendingPath, offset, 60*time.Second); err != nil {
		if err := media.Thumbnail(ctx, target, headers, caFile, tlsSkipVerify, s.ffmpegPath, pendingPath, 0, 60*time.Second); err != nil {
			return Video{}, err
		}
	}
	data, err := os.ReadFile(pendingPath)
	if err != nil {
		return Video{}, err
	}
	return s.saveCover(ctx, videoID, "image/jpeg", data)
}

// SetFFmpegPath 設定封面擷取使用的 FFmpeg；空值時保留預設 ffmpeg。
func (s *Store) SetFFmpegPath(path string) {
	if strings.TrimSpace(path) != "" {
		s.ffmpegPath = strings.TrimSpace(path)
	}
}

func (s *Store) coverDirectory() string {
	return filepath.Join(s.dataDirectory, "covers")
}

func (s *Store) coverPath(videoID string) string {
	return filepath.Join(s.coverDirectory(), videoID)
}

func (s *Store) saveCover(ctx context.Context, videoID, mime string, data []byte) (Video, error) {
	if len(data) == 0 || len(data) > maxCoverBytes {
		return Video{}, fmt.Errorf("封面大小必須介於 1 byte 到 8 MiB")
	}
	allowed := map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}
	if !allowed[mime] {
		return Video{}, fmt.Errorf("封面格式只支援 JPEG、PNG 或 WebP")
	}
	if err := os.MkdirAll(s.coverDirectory(), 0755); err != nil {
		return Video{}, err
	}
	pending, err := os.CreateTemp(s.coverDirectory(), "."+videoID+".pending-*")
	if err != nil {
		return Video{}, err
	}
	tmp := pending.Name()
	cleanup := true
	defer func() {
		pending.Close()
		if cleanup {
			os.Remove(tmp)
		}
	}()
	if err := pending.Chmod(0644); err != nil {
		return Video{}, err
	}
	if _, err := pending.Write(data); err != nil {
		return Video{}, err
	}
	if err := pending.Close(); err != nil {
		return Video{}, err
	}
	target := s.coverPath(videoID)
	_ = os.Remove(target)
	if err := os.Rename(tmp, target); err != nil {
		return Video{}, err
	}
	cleanup = false
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, "UPDATE videos SET cover_updated_at=?,cover_mime=? WHERE id=?", updated, mime, videoID)
	if err != nil {
		return Video{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Video{}, err
	}
	if affected == 0 {
		os.Remove(target)
		return Video{}, errors.New("找不到影片")
	}
	s.publish("jobs")
	return s.video(videoID)
}

func (s *Store) deleteCover(ctx context.Context, videoID string) (Video, error) {
	if err := os.Remove(s.coverPath(videoID)); err != nil && !os.IsNotExist(err) {
		return Video{}, err
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE videos SET cover_updated_at='',cover_mime='' WHERE id=?", videoID); err != nil {
		return Video{}, err
	}
	s.publish("jobs")
	return s.video(videoID)
}

func (s *Store) cover(videoID string) (string, string, string, error) {
	var mime, updated string
	err := s.db.QueryRow("SELECT cover_mime,cover_updated_at FROM videos WHERE id=?", videoID).Scan(&mime, &updated)
	if err != nil {
		return "", "", "", err
	}
	if mime == "" || updated == "" {
		return "", "", "", os.ErrNotExist
	}
	return s.coverPath(videoID), mime, updated, nil
}
