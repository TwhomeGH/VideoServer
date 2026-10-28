package library

import (
	"context"
	"time"
)

const coverFailureCooldown = 10 * time.Minute

// EnsureCover 將缺少封面的影片排入單一背景 worker；重複請求會去重。
func (s *Store) EnsureCover(videoID string) {
	s.coverMu.Lock()
	if s.coverPending[videoID] {
		s.coverMu.Unlock()
		return
	}
	if failedAt, exists := s.coverFailed[videoID]; exists && time.Since(failedAt) < coverFailureCooldown {
		s.coverMu.Unlock()
		return
	}
	s.coverPending[videoID] = true
	s.coverMu.Unlock()
	select {
	case s.coverWake <- videoID:
	default:
		s.coverMu.Lock()
		delete(s.coverPending, videoID)
		s.coverMu.Unlock()
	}
}

// RunCoverWorker 一次只產生一張封面，避免 FFmpeg 同時掃描多部影片。
func (s *Store) RunCoverWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case videoID := <-s.coverWake:
			_, err := s.GenerateCover(ctx, videoID)
			s.coverMu.Lock()
			delete(s.coverPending, videoID)
			if err != nil {
				s.coverFailed[videoID] = time.Now()
			} else {
				delete(s.coverFailed, videoID)
			}
			s.coverMu.Unlock()
		}
	}
}

// RunCoverBackfill 每個 interval 找一部缺少封面的影片排入 worker。
func (s *Store) RunCoverBackfill(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		s.enqueueNextCover(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Store) enqueueNextCover(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM videos WHERE cover_updated_at='' ORDER BY imported_at,id LIMIT 100")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var videoID string
		if err := rows.Scan(&videoID); err != nil {
			return
		}
		s.coverMu.Lock()
		pending := s.coverPending[videoID]
		failedAt, failed := s.coverFailed[videoID]
		s.coverMu.Unlock()
		if pending || (failed && time.Since(failedAt) < coverFailureCooldown) {
			continue
		}
		s.EnsureCover(videoID)
		return
	}
}
