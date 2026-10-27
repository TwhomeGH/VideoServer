package library

import (
	"context"
	"database/sql"
	"time"
)

// DiscoveredMedia 是 index-only 掃描找到、但尚未正式匯入的影片。
type DiscoveredMedia struct {
	PoolID       string `json:"poolId"`
	PoolName     string `json:"poolName"`
	Provider     string `json:"provider"`
	RelativePath string `json:"relativePath"`
	Size         int64  `json:"size"`
	ModifiedAt   int64  `json:"modifiedAt"`
	FirstSeen    string `json:"firstSeen"`
	LastSeen     string `json:"lastSeen"`
}

type DiscoveredPage struct {
	Items    []DiscoveredMedia `json:"items"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
}

func (s *Store) upsertDiscovered(ctx context.Context, poolID, relative string, size int64, modified time.Time, at time.Time) (bool, error) {
	key := fileKey(poolID, relative)
	var existingSize, existingModified int64
	var status string
	err := s.db.QueryRowContext(ctx, "SELECT size,modified_at,status FROM discovered_media WHERE pool_id=? AND path_key=?", poolID, key).Scan(&existingSize, &existingModified, &status)
	if err == nil && status == "found" && existingSize == size && existingModified == modified.UnixMilli() {
		return false, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	now := at.UTC().Format(time.RFC3339Nano)
	if at.IsZero() {
		now = timestamp()
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO discovered_media(pool_id,path_key,relative_path,size,modified_at,first_seen,last_seen,status)
VALUES(?,?,?,?,?,?,?,'found')
ON CONFLICT(pool_id,path_key) DO UPDATE SET
 relative_path=excluded.relative_path,size=excluded.size,modified_at=excluded.modified_at,
 last_seen=excluded.last_seen,status='found'`,
		poolID, key, relative, size, modified.UnixMilli(), now, now)
	return true, err
}

func (s *Store) hasDiscovered(ctx context.Context, poolID, relative string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM discovered_media WHERE pool_id=? AND path_key=? AND status=? LIMIT 1", poolID, fileKey(poolID, relative), "found").Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) DiscoveredExists(ctx context.Context, poolID, relative string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM discovered_media WHERE pool_id=? AND path_key=? AND status='found' LIMIT 1", poolID, fileKey(poolID, relative)).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) markDiscoveredMissing(ctx context.Context, poolID string, seen map[string]bool) error {
	rows, err := s.db.QueryContext(ctx, "SELECT path_key FROM discovered_media WHERE pool_id=? AND status='found'", poolID)
	if err != nil {
		return err
	}
	var missing []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		if !seen[key] {
			missing = append(missing, key)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, key := range missing {
		if _, err := s.db.ExecContext(ctx, "UPDATE discovered_media SET status='missing' WHERE pool_id=? AND path_key=?", poolID, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListDiscovered(ctx context.Context, poolID, query string, page, pageSize int) (DiscoveredPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	where := "d.status='found' AND v.id IS NULL"
	args := []any{}
	if poolID != "" {
		where += " AND d.pool_id=?"
		args = append(args, poolID)
	}
	if query != "" {
		where += " AND d.relative_path LIKE ?"
		args = append(args, "%"+query+"%")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM discovered_media d LEFT JOIN videos v ON v.pool_id=d.pool_id AND v.relative_path=d.relative_path WHERE "+where, args...).Scan(&total); err != nil {
		return DiscoveredPage{}, err
	}
	offset := (page - 1) * pageSize
	rows, err := s.db.QueryContext(ctx, `
SELECT d.pool_id,COALESCE(p.name,''),p.provider,d.relative_path,d.size,d.modified_at,d.first_seen,d.last_seen
FROM discovered_media d
JOIN pools p ON p.id=d.pool_id
LEFT JOIN videos v ON v.pool_id=d.pool_id AND v.relative_path=d.relative_path
WHERE `+where+`
ORDER BY d.last_seen DESC, d.relative_path
LIMIT ? OFFSET ?`, append(args, pageSize, offset)...)
	if err != nil {
		return DiscoveredPage{}, err
	}
	defer rows.Close()
	result := DiscoveredPage{Items: []DiscoveredMedia{}, Total: total, Page: page, PageSize: pageSize}
	for rows.Next() {
		item := DiscoveredMedia{}
		if err := rows.Scan(&item.PoolID, &item.PoolName, &item.Provider, &item.RelativePath, &item.Size, &item.ModifiedAt, &item.FirstSeen, &item.LastSeen); err != nil {
			return DiscoveredPage{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
