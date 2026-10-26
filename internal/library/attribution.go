package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func saveAttribution(ctx context.Context, tx *sql.Tx, id string, m Metadata) error {
	creators, err := json.Marshal(m.Creators)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO video_attribution VALUES(?,?,?,?,?,?,?,?)
ON CONFLICT(video_id) DO UPDATE SET work_type=excluded.work_type,creators_json=excluded.creators_json,
source_url=excluded.source_url,source_platform=excluded.source_platform,source_pending=excluded.source_pending,
repost_note=excluded.repost_note,language=excluded.language`, id, m.WorkType, string(creators), m.SourceURL, m.SourcePlatform, m.SourcePending, m.RepostNote, m.Language)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM video_tags WHERE video_id=?", id); err != nil {
		return err
	}
	for i, tag := range m.Tags {
		if _, err = tx.ExecContext(ctx, "INSERT INTO video_tags VALUES(?,?,?,?)", id, strings.ToLower(tag), tag, i); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadAttribution(v *Video) error {
	v.WorkType = "unknown"
	v.Creators = []Creator{}
	v.Tags = []string{}
	var creators string
	err := s.db.QueryRow(`SELECT work_type,creators_json,source_url,source_platform,source_pending,repost_note,language FROM video_attribution WHERE video_id=?`, v.ID).Scan(&v.WorkType, &creators, &v.SourceURL, &v.SourcePlatform, &v.SourcePending, &v.RepostNote, &v.Language)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if err := json.Unmarshal([]byte(creators), &v.Creators); err != nil {
			return err
		}
	}
	rows, err := s.db.Query("SELECT label FROM video_tags WHERE video_id=? ORDER BY position", v.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return err
		}
		v.Tags = append(v.Tags, tag)
	}
	return rows.Err()
}

// UpdateMetadata 以完整描述取代 id 的描述與來源資訊，不變更檔案、識別或雜湊。
// 欄位驗證或交易失敗不會留下部分更新；未知影片回傳 sql.ErrNoRows。
func (s *Store) UpdateMetadata(ctx context.Context, id, title string, metadata Metadata) (Video, error) {
	var empty Video
	title = strings.TrimSpace(title)
	if title == "" {
		return empty, fmt.Errorf("標題不可為空")
	}
	details, err := metadata.normalize()
	if err != nil {
		return empty, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE videos SET title=?,description=?,category=?,adult=?,license=? WHERE id=?`, title, details.Description, details.Category, details.Adult, details.License, id)
	if err != nil {
		return empty, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return empty, err
	}
	if count == 0 {
		return empty, sql.ErrNoRows
	}
	if err := saveAttribution(ctx, tx, id, details); err != nil {
		return empty, err
	}
	if err := tx.Commit(); err != nil {
		return empty, err
	}
	video, err := s.video(id)
	if err == nil {
		if file, e := s.openVideo(video); e == nil {
			video.Available = true
			file.Close()
		}
	}
	return video, err
}
