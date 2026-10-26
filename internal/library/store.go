// Package library 管理 Video Node 的儲存池與本機影片索引。
package library

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TwhomeGH/VideoServer/internal/media"

	_ "modernc.org/sqlite"
)

// Store 持有單一節點的 SQLite 索引；使用完畢需呼叫 Close。
type Store struct {
	db      *sql.DB
	nodeID  string
	scanner *Scanner
	queue   *ImportQueue
}

// Pool 表示已註冊的儲存根目錄；Online 每次查詢時重新檢查。
type Pool struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Online bool   `json:"online"`
}

// Video 表示一部影片與第一份原始檔案副本，路徑不對外公開。
type Video struct {
	ID            string `json:"id"`
	PublicationID string `json:"publicationId"`
	ReplicaID     string `json:"replicaId"`
	PoolID        string `json:"poolId"`
	Title         string `json:"title"`
	Metadata
	FileHash     string      `json:"fileHash"`
	SourceHash   string      `json:"sourceHash"`
	Size         int64       `json:"size"`
	ImportedAt   string      `json:"importedAt"`
	Available    bool        `json:"available"`
	Media        *media.Info `json:"media,omitempty"`
	relativePath string
	modifiedAt   int64
}

func identifier() string { return fmt.Sprintf("%x", mustRandom()) }

func mustRandom() []byte {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return value
}

// Open 建立 directory 與 SQLite schema，並保存可跨重新啟動沿用的 NodeID。
// 無法建立資料庫或套用遷移時回傳錯誤，不以空資料繼續啟動。
func Open(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(directory, "library.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("CREATE TABLE IF NOT EXISTS schema_version(version INTEGER PRIMARY KEY)"); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRow("SELECT COALESCE(MAX(version),0) FROM schema_version").Scan(&version); err != nil {
		return err
	}
	if version > 4 {
		return fmt.Errorf("資料庫版本 %d 高於目前支援的版本 4", version)
	}
	if _, err := tx.Exec(`
 CREATE TABLE IF NOT EXISTS schema_version(version INTEGER PRIMARY KEY);
 CREATE TABLE IF NOT EXISTS metadata(key TEXT PRIMARY KEY, value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS pools(id TEXT PRIMARY KEY, name TEXT NOT NULL, path TEXT NOT NULL UNIQUE);
 CREATE TABLE IF NOT EXISTS videos(
 id TEXT PRIMARY KEY, publication_id TEXT NOT NULL UNIQUE, replica_id TEXT NOT NULL UNIQUE,
 pool_id TEXT NOT NULL REFERENCES pools(id), title TEXT NOT NULL, relative_path TEXT NOT NULL,
 file_hash TEXT NOT NULL, source_hash TEXT NOT NULL, size INTEGER NOT NULL,
 modified_at INTEGER NOT NULL, imported_at TEXT NOT NULL, UNIQUE(pool_id, relative_path));
 INSERT OR IGNORE INTO schema_version VALUES(1);`); err != nil {
		return err
	}
	if version < 2 {
		if _, err := tx.Exec(`
ALTER TABLE videos ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE videos ADD COLUMN category TEXT NOT NULL DEFAULT '';
ALTER TABLE videos ADD COLUMN adult INTEGER NOT NULL DEFAULT 0 CHECK(adult IN (0,1));
ALTER TABLE videos ADD COLUMN license TEXT NOT NULL DEFAULT '';
INSERT INTO schema_version VALUES(2);`); err != nil {
			return err
		}
	}
	if version < 3 {
		if _, err := tx.Exec(`
CREATE TABLE video_attribution(
 video_id TEXT PRIMARY KEY REFERENCES videos(id), work_type TEXT NOT NULL,
 creators_json TEXT NOT NULL, source_url TEXT NOT NULL, source_platform TEXT NOT NULL,
 source_pending INTEGER NOT NULL, repost_note TEXT NOT NULL, language TEXT NOT NULL);
CREATE TABLE video_tags(
 video_id TEXT NOT NULL REFERENCES videos(id), tag_key TEXT NOT NULL, label TEXT NOT NULL,
 position INTEGER NOT NULL, PRIMARY KEY(video_id,tag_key));
CREATE INDEX video_tags_lookup ON video_tags(tag_key,video_id);
INSERT INTO schema_version VALUES(3);`); err != nil {
			return err
		}
	}
	if version < 4 {
		if _, err := tx.Exec(`CREATE TABLE video_media(video_id TEXT PRIMARY KEY REFERENCES videos(id), details_json TEXT NOT NULL);
CREATE TABLE import_jobs(
 id TEXT PRIMARY KEY,pool_id TEXT NOT NULL REFERENCES pools(id),relative_path TEXT NOT NULL,path_key TEXT NOT NULL,
 title TEXT NOT NULL,metadata_json TEXT NOT NULL,source TEXT NOT NULL,
 status TEXT NOT NULL,phase TEXT NOT NULL,bytes_processed INTEGER NOT NULL,total_bytes INTEGER NOT NULL,
 video_id TEXT NOT NULL,error TEXT NOT NULL,attempts INTEGER NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE UNIQUE INDEX import_jobs_path ON import_jobs(pool_id,path_key);
CREATE INDEX import_jobs_pending ON import_jobs(status,created_at);
INSERT INTO schema_version VALUES(4);`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("INSERT OR IGNORE INTO metadata VALUES('nodeId', ?)", identifier()); err != nil {
		return err
	}
	if err := tx.QueryRow("SELECT value FROM metadata WHERE key='nodeId'").Scan(&s.nodeID); err != nil {
		return err
	}
	return tx.Commit()
}

// Close 釋放資料庫連線，回傳資料庫關閉錯誤。
func (s *Store) Close() error { return s.db.Close() }

// Pools 回傳所有已註冊儲存池，離線池保留資料而不刪除。
func (s *Store) Pools() ([]Pool, error) {
	rows, err := s.db.Query("SELECT id,name,path FROM pools ORDER BY name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Pool{}
	for rows.Next() {
		var pool Pool
		if err := rows.Scan(&pool.ID, &pool.Name, &pool.Path); err != nil {
			return nil, err
		}
		info, err := os.Stat(pool.Path)
		pool.Online = err == nil && info.IsDir()
		result = append(result, pool)
	}
	return result, rows.Err()
}

// AddPool 註冊現存目錄，不移動或刪除原始檔案；重複目錄會回傳錯誤。
func (s *Store) AddPool(name, path string) (Pool, error) {
	var pool Pool
	if strings.TrimSpace(name) == "" || !filepath.IsAbs(path) {
		return pool, fmt.Errorf("請提供名稱與伺服器上的絕對目錄路徑")
	}
	path, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return pool, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return pool, err
	}
	if !info.IsDir() {
		return pool, fmt.Errorf("儲存池必須是目錄")
	}
	pool = Pool{ID: identifier(), Name: strings.TrimSpace(name), Path: path, Online: true}
	_, err = s.db.Exec("INSERT INTO pools(id,name,path) VALUES(?,?,?)", pool.ID, pool.Name, pool.Path)
	return pool, err
}

// resolve 僅允許儲存池內的相對檔案路徑，解析連結後再次檢查邊界。
func resolve(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" {
		return "", fmt.Errorf("請使用儲存池內的相對路徑")
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(canonicalRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("檔案必須位於指定儲存池內")
	}
	return path, nil
}
