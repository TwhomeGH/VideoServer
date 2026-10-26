package library

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// VideoQuery 限制每次清單查詢的範圍；Tag、分類與儲存池採精確比對。
type VideoQuery struct {
	Page, PageSize                             int
	Search, Tag, Category, PoolID, Adult, Sort string
}

// VideoPage 回傳本頁影片、篩選筆數與整個影片庫筆數；可用狀態只檢查本頁。
type VideoPage struct {
	Videos      []Video `json:"videos"`
	Page        int     `json:"page"`
	PageSize    int     `json:"pageSize"`
	Total       int     `json:"total"`
	TotalVideos int     `json:"totalVideos"`
}

func parseVideoQuery(values url.Values) (VideoQuery, error) {
	q := VideoQuery{Page: 1, PageSize: 24, Search: strings.TrimSpace(values.Get("q")),
		Tag: strings.TrimSpace(values.Get("tag")), Category: strings.TrimSpace(values.Get("category")),
		PoolID: strings.TrimSpace(values.Get("poolId")), Adult: values.Get("adult"), Sort: values.Get("sort")}
	for _, field := range []struct {
		key    string
		target *int
	}{{"page", &q.Page}, {"pageSize", &q.PageSize}} {
		if raw, exists := values[field.key]; exists {
			if len(raw) != 1 {
				return q, fmt.Errorf("分頁參數不可重複")
			}
			value, err := strconv.Atoi(raw[0])
			if err != nil || value < 1 || value > 1000000 {
				return q, fmt.Errorf("分頁參數無效")
			}
			*field.target = value
		}
	}
	if err := q.validate(); err != nil {
		return q, err
	}
	return q, nil
}

func (q VideoQuery) validate() error {
	if q.Page < 1 || q.Page > 1000000 || q.PageSize < 1 || q.PageSize > 100 {
		return fmt.Errorf("頁碼或每頁筆數無效（每頁最多 100 筆）")
	}
	for _, value := range []string{q.Search, q.Tag, q.Category, q.PoolID} {
		if utf8.RuneCountInString(value) > 200 {
			return fmt.Errorf("查詢條件最多 200 字")
		}
	}
	if q.Adult != "" && q.Adult != "true" && q.Adult != "false" {
		return fmt.Errorf("18+ 篩選值無效")
	}
	if q.Sort != "" && q.Sort != "newest" && q.Sort != "oldest" && q.Sort != "title" {
		return fmt.Errorf("排序條件無效")
	}
	return nil
}

// QueryVideos 以 q 的條件查詢 ctx 範圍內的一頁影片；空頁碼範圍會收斂至最後一頁。
// 所有文字值使用 SQL 參數，搜尋的百分號與底線視為普通文字。
func (s *Store) QueryVideos(ctx context.Context, q VideoQuery) (VideoPage, error) {
	result := VideoPage{Page: q.Page, PageSize: q.PageSize}
	if err := q.validate(); err != nil {
		return result, err
	}
	conditions := []string{"1=1"}
	args := []any{}
	if q.Search != "" {
		conditions = append(conditions, "(instr(lower(title),lower(?))>0 OR instr(lower(description),lower(?))>0)")
		args = append(args, q.Search, q.Search)
	}
	if q.Tag != "" {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM video_tags WHERE video_id=videos.id AND tag_key=?)")
		args = append(args, strings.ToLower(q.Tag))
	}
	for _, field := range []struct{ column, value string }{{"category", q.Category}, {"pool_id", q.PoolID}} {
		if field.value != "" {
			conditions = append(conditions, field.column+"=?")
			args = append(args, field.value)
		}
	}
	if q.Adult != "" {
		conditions = append(conditions, "adult=?")
		args = append(args, q.Adult == "true")
	}
	where := " WHERE " + strings.Join(conditions, " AND ")
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM videos").Scan(&result.TotalVideos); err != nil {
		return result, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM videos"+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	last := max(1, (result.Total+q.PageSize-1)/q.PageSize)
	result.Page = min(q.Page, last)
	order := "imported_at DESC,id"
	if q.Sort == "oldest" {
		order = "imported_at ASC,id"
	}
	if q.Sort == "title" {
		order = "title COLLATE NOCASE,id"
	}
	args = append(args, q.PageSize, (result.Page-1)*q.PageSize)
	videos, err := s.listVideos(ctx, "SELECT "+videoColumns+" FROM videos"+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
	result.Videos = videos
	return result, err
}
