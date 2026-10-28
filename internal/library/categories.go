package library

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Category 是可管理的影片分類；videos.category 仍保留名稱以相容既有查詢。
type Category struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	VideoCount int    `json:"videoCount"`
}

func normalizeCategoryName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("分類名稱不可為空")
	}
	if len([]rune(name)) > 80 {
		return "", fmt.Errorf("分類名稱最多 80 字")
	}
	return name, nil
}

func (s *Store) Categories(ctx context.Context) ([]Category, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id,c.name,COUNT(v.id)
FROM categories c
LEFT JOIN videos v ON lower(v.category)=lower(c.name)
GROUP BY c.id,c.name
ORDER BY c.name COLLATE NOCASE,c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Category{}
	for rows.Next() {
		var category Category
		if err := rows.Scan(&category.ID, &category.Name, &category.VideoCount); err != nil {
			return nil, err
		}
		result = append(result, category)
	}
	return result, rows.Err()
}

func (s *Store) AddCategory(ctx context.Context, name string) (Category, error) {
	name, err := normalizeCategoryName(name)
	if err != nil {
		return Category{}, err
	}
	category := Category{ID: identifier(), Name: name}
	_, err = s.db.ExecContext(ctx, "INSERT INTO categories(id,name,created_at) VALUES(?,?,?)", category.ID, category.Name, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Category{}, fmt.Errorf("分類已存在或無法建立")
	}
	s.publish("jobs")
	return category, nil
}

func (s *Store) RenameCategory(ctx context.Context, id, name string) (Category, error) {
	name, err := normalizeCategoryName(name)
	if err != nil {
		return Category{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Category{}, err
	}
	defer tx.Rollback()
	var oldName string
	if err := tx.QueryRowContext(ctx, "SELECT name FROM categories WHERE id=?", id).Scan(&oldName); err != nil {
		return Category{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE categories SET name=? WHERE id=?", name, id); err != nil {
		return Category{}, fmt.Errorf("分類名稱已存在或無法更新")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE videos SET category=? WHERE category=?", name, oldName); err != nil {
		return Category{}, err
	}
	if err := tx.Commit(); err != nil {
		return Category{}, err
	}
	s.publish("jobs")
	return Category{ID: id, Name: name}, nil
}

func (s *Store) DeleteCategory(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var name string
	if err := tx.QueryRowContext(ctx, "SELECT name FROM categories WHERE id=?", id).Scan(&name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM categories WHERE id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE videos SET category='' WHERE category=?", name); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.publish("jobs")
	return nil
}

func (s *Store) ensureCategory(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO categories(id,name,created_at) VALUES(?,?,?)", identifier(), name, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
