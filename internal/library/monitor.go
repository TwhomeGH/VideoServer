package library

import "context"

// WorkStatus 包含全佇列狀態筆數及最多十筆執行／等待工作，不依最近一百筆推算總數。
type WorkStatus struct {
	Counts map[string]int `json:"counts"`
	Active []ImportJob    `json:"active"`
}

// WorkStatus 讀取 ctx 範圍內的背景工作快照；執行中的工作優先列出。
func (q *ImportQueue) WorkStatus(ctx context.Context) (WorkStatus, error) {
	result := WorkStatus{Counts: map[string]int{"queued": 0, "running": 0, "failed": 0, "completed": 0}, Active: []ImportJob{}}
	rows, err := q.store.db.QueryContext(ctx, "SELECT status,COUNT(*) FROM import_jobs GROUP BY status")
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return result, err
		}
		result.Counts[status] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = q.store.db.QueryContext(ctx, "SELECT "+jobColumns+" FROM import_jobs WHERE status IN ('queued','running') ORDER BY CASE status WHEN 'running' THEN 0 ELSE 1 END,created_at,id LIMIT 10")
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return result, err
		}
		result.Active = append(result.Active, job)
	}
	return result, rows.Err()
}
