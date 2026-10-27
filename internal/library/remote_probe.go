package library

import (
	"context"
	"encoding/json"

	"github.com/TwhomeGH/VideoServer/internal/media"
)

func (q *ImportQueue) processRemoteProbe(ctx context.Context, job ImportJob, video Video) error {
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
	info, err := media.ProbeRemote(ctx, target.String(), username, password, pool.TLSSkipVerify, q.ffprobePath, q.probeTimeout)
	if err != nil {
		return err
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
	if _, err := tx.ExecContext(ctx, "INSERT INTO video_media VALUES(?,?) ON CONFLICT(video_id) DO UPDATE SET details_json=excluded.details_json", video.ID, string(payload)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE import_jobs SET status='completed',phase='completed',bytes_processed=?,error='',updated_at=? WHERE id=? AND status='running'", video.Size, timestamp(), job.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	q.store.publish("jobs")
	return nil
}
