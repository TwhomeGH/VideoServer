// Package library 管理 Video Node 的儲存池與本機影片索引。
package library

import (
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/media"

	_ "modernc.org/sqlite"
)

var (
	ErrPoolDisabled  = errors.New("儲存池已停用")
	ErrPoolOffline   = errors.New("儲存池目前離線")
	ErrSourceMissing = errors.New("來源檔案遺失")
	ErrSourceChanged = errors.New("來源檔案已變更")
)

// Store 持有單一節點的 SQLite 索引；使用完畢需呼叫 Close。
type Store struct {
	db                      *sql.DB
	nodeID                  string
	scanner                 *Scanner
	queue                   *ImportQueue
	hashLimitBytesPerSecond int64
	publisher               Publisher
	videoFormats            *media.VideoFormats
	dataDirectory           string
	secretMu                sync.Mutex
}

// Publisher 接收資料變更通知；實作可以是非阻塞的事件 hub。
type Publisher interface {
	Publish(eventType string)
}

func (s *Store) SetPublisher(publisher Publisher) {
	s.publisher = publisher
}

func (s *Store) SetVideoFormats(formats *media.VideoFormats) {
	s.videoFormats = formats
}

func (s *Store) supportedVideo(path string) bool {
	return s.videoFormats.Supports(path)
}

func (s *Store) publish(eventType string) {
	if s.publisher != nil {
		s.publisher.Publish(eventType)
	}
}

// Pool 表示已註冊的儲存根目錄；Online 每次查詢時重新檢查，Enabled 是管理者的開關。
type Pool struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Path              string `json:"path"`
	Online            bool   `json:"online"`
	Enabled           bool   `json:"enabled"`
	Filesystem        string `json:"filesystem,omitempty"`
	VolumeID          string `json:"volumeId,omitempty"`
	VolumeStatus      string `json:"volumeStatus,omitempty"`
	LastActivityAt    string `json:"lastActivityAt,omitempty"`
	ScanExcludeSystem bool   `json:"scanExcludeSystem"`
	IndexOnly         bool   `json:"indexOnly"`
	Provider          string `json:"provider"`
	RemoteURL         string `json:"remoteUrl,omitempty"`
	RemoteUser        string `json:"remoteUser,omitempty"`
	RemoteRoot        string `json:"remoteRoot,omitempty"`
	TLSSkipVerify     bool   `json:"tlsSkipVerify"`
	TLSCASet          bool   `json:"tlsCaSet"`
	TLSCA             string `json:"-"`
	WholeDisk         bool   `json:"wholeDisk"`
	volumeRelative    string
}

// Video 表示一部影片與第一份原始檔案副本，路徑不對外公開。
type Video struct {
	ID            string `json:"id"`
	PublicationID string `json:"publicationId"`
	ReplicaID     string `json:"replicaId"`
	PoolID        string `json:"poolId"`
	Title         string `json:"title"`
	Metadata
	FileHash          string      `json:"fileHash"`
	SourceHash        string      `json:"sourceHash"`
	Size              int64       `json:"size"`
	ImportedAt        string      `json:"importedAt"`
	Available         bool        `json:"available"`
	UnavailableReason string      `json:"unavailableReason,omitempty"`
	Provider          string      `json:"provider"`
	HashStatus        string      `json:"hashStatus"`
	Media             *media.Info `json:"media,omitempty"`
	relativePath      string
	modifiedAt        int64
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
	store := &Store{db: db, videoFormats: media.DefaultVideoFormats(), dataDirectory: directory}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	_ = store.backfillPoolMetadata()
	return store, nil
}

// migrate 依 schema_version 線性升級 SQLite，與 buildinfo.Version、protocol.Current 無關。
// 新資料庫 version=0，會依序執行所有 migration；舊資料庫只補缺少的版本。
// 舊程式遇到更高的 schema（目前 >9）會拒絕啟動，避免降級寫入。
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
	if version > 14 {
		return fmt.Errorf("資料庫版本 %d 高於目前支援的版本 14", version)
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
	if version < 5 {
		columns := []struct {
			name       string
			definition string
		}{
			{"volume_id", "TEXT NOT NULL DEFAULT ''"},
			{"volume_relative", "TEXT NOT NULL DEFAULT ''"},
			{"filesystem", "TEXT NOT NULL DEFAULT ''"},
			{"enabled", "INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1))"},
		}
		for _, column := range columns {
			exists, err := columnExists(tx, "pools", column.name)
			if err != nil {
				return err
			}
			if !exists {
				if _, err := tx.Exec("ALTER TABLE pools ADD COLUMN " + column.name + " " + column.definition); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec("INSERT INTO schema_version VALUES(5)"); err != nil {
			return err
		}
	}
	if version < 6 {
		if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS backfill_usage(
 day TEXT PRIMARY KEY, videos INTEGER NOT NULL DEFAULT 0, bytes INTEGER NOT NULL DEFAULT 0);
INSERT INTO schema_version VALUES(6);`); err != nil {
			return err
		}
	}
	if version < 7 {
		exists, err := columnExists(tx, "pools", "last_activity_at")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec("ALTER TABLE pools ADD COLUMN last_activity_at TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`
UPDATE pools SET last_activity_at = COALESCE(
 (SELECT MAX(imported_at) FROM videos WHERE videos.pool_id = pools.id), ?)
WHERE last_activity_at = ''`, timestamp()); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO schema_version VALUES(7)"); err != nil {
			return err
		}
	}

	if version < 8 {
		exists, err := columnExists(tx, "pools", "scan_exclude_system")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec("ALTER TABLE pools ADD COLUMN scan_exclude_system INTEGER NOT NULL DEFAULT 1 CHECK(scan_exclude_system IN (0,1))"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("INSERT INTO schema_version VALUES(8)"); err != nil {
			return err
		}
	}
	if version < 9 {
		exists, err := columnExists(tx, "pools", "index_only")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec("ALTER TABLE pools ADD COLUMN index_only INTEGER NOT NULL DEFAULT 0 CHECK(index_only IN (0,1))"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS discovered_media(
 pool_id TEXT NOT NULL REFERENCES pools(id), path_key TEXT NOT NULL, relative_path TEXT NOT NULL,
 size INTEGER NOT NULL, modified_at INTEGER NOT NULL, first_seen TEXT NOT NULL, last_seen TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'found' CHECK(status IN ('found','missing')),
 PRIMARY KEY(pool_id,path_key));
CREATE INDEX IF NOT EXISTS discovered_media_pool ON discovered_media(pool_id,status,last_seen);
INSERT INTO schema_version VALUES(9);`); err != nil {
			return err
		}
	}
	if version < 10 {
		columns := []struct {
			name       string
			definition string
		}{
			{"provider", "TEXT NOT NULL DEFAULT 'local'"},
			{"remote_url", "TEXT NOT NULL DEFAULT ''"},
			{"remote_user", "TEXT NOT NULL DEFAULT ''"},
			{"remote_root", "TEXT NOT NULL DEFAULT ''"},
		}
		for _, column := range columns {
			exists, err := columnExists(tx, "pools", column.name)
			if err != nil {
				return err
			}
			if !exists {
				if _, err := tx.Exec("ALTER TABLE pools ADD COLUMN " + column.name + " " + column.definition); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec("INSERT INTO schema_version VALUES(10)"); err != nil {
			return err
		}
	}
	if version < 11 {
		columns := []struct {
			name       string
			definition string
		}{
			{"provider", "TEXT NOT NULL DEFAULT 'local'"},
			{"hash_status", "TEXT NOT NULL DEFAULT 'verified'"},
		}
		for _, column := range columns {
			exists, err := columnExists(tx, "videos", column.name)
			if err != nil {
				return err
			}
			if !exists {
				if _, err := tx.Exec("ALTER TABLE videos ADD COLUMN " + column.name + " " + column.definition); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec("INSERT INTO schema_version VALUES(11)"); err != nil {
			return err
		}
	}
	if version < 12 {
		exists, err := columnExists(tx, "pools", "tls_skip_verify")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec("ALTER TABLE pools ADD COLUMN tls_skip_verify INTEGER NOT NULL DEFAULT 0"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("INSERT INTO schema_version VALUES(12)"); err != nil {
			return err
		}
	}
	if version < 13 {
		exists, err := columnExists(tx, "pools", "tls_ca")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec("ALTER TABLE pools ADD COLUMN tls_ca TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("INSERT INTO schema_version VALUES(13)"); err != nil {
			return err
		}
	}
	if version < 14 {
		exists, err := columnExists(tx, "discovered_media", "etag")
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec("ALTER TABLE discovered_media ADD COLUMN etag TEXT NOT NULL DEFAULT ''"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("INSERT INTO schema_version VALUES(14)"); err != nil {
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

// columnExists 讓 schema 遷移可在測試重設版本或部分欄位已存在時安全重跑。
func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Close 釋放資料庫連線，回傳資料庫關閉錯誤。
func (s *Store) Close() error { return s.db.Close() }

// Pools 回傳所有已註冊儲存池，離線或被停用的池保留資料而不刪除。
func (s *Store) Pools() ([]Pool, error) {
	rows, err := s.db.Query("SELECT id,name,path,volume_id,volume_relative,filesystem,enabled,last_activity_at,scan_exclude_system,index_only,provider,remote_url,remote_user,remote_root,tls_skip_verify,tls_ca FROM pools ORDER BY name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Pool{}
	for rows.Next() {
		var pool Pool
		var enabled, excludeSystem, indexOnly int
		if err := rows.Scan(&pool.ID, &pool.Name, &pool.Path, &pool.VolumeID, &pool.volumeRelative, &pool.Filesystem, &enabled, &pool.LastActivityAt, &excludeSystem, &indexOnly, &pool.Provider, &pool.RemoteURL, &pool.RemoteUser, &pool.RemoteRoot, &pool.TLSSkipVerify, &pool.TLSCA); err != nil {
			return nil, err
		}
		pool.Enabled = enabled != 0
		pool.TLSCASet = pool.TLSCA != ""
		pool.ScanExcludeSystem = excludeSystem != 0
		pool.IndexOnly = indexOnly != 0
		if pool.Provider == "webdav" {
			pool.Online = true
			pool.Filesystem = "WebDAV"
			pool.VolumeStatus = "unsupported"
			pool.WholeDisk = false
		} else {
			pool.WholeDisk = isWholeDisk(pool.Path)
			applyVolumeStatus(&pool)
		}
		if effective, err := effectivePoolPath(pool.Path, pool.VolumeID, pool.volumeRelative); err == nil {
			if info, statErr := os.Stat(effective); statErr == nil && info.IsDir() {
				pool.Online = true
			}
		}
		result = append(result, pool)
	}
	return result, rows.Err()
}

// AddPool 註冊現存目錄，不移動或刪除原始檔案；重複目錄會回傳錯誤。
// Windows 額外保存 volume GUID 與相對路徑，之後盤符改變仍可解析到同一個磁碟區。
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
	volumeID, volumeRelative, filesystem := volumeDetailsFor(path)
	pool = Pool{
		ID: identifier(), Name: strings.TrimSpace(name), Path: path, Online: true, Enabled: true,
		Filesystem: filesystem, VolumeID: volumeID, volumeRelative: volumeRelative, LastActivityAt: timestamp(), ScanExcludeSystem: true, IndexOnly: false, Provider: "local", WholeDisk: isWholeDisk(path),
	}
	applyVolumeStatus(&pool)
	_, err = s.db.Exec("INSERT INTO pools(id,name,path,volume_id,volume_relative,filesystem,enabled,last_activity_at,scan_exclude_system,index_only,provider,remote_url,remote_user,remote_root,tls_skip_verify,tls_ca) VALUES(?,?,?,?,?,?,1,?,1,0,'local','','','',0,'')", pool.ID, pool.Name, pool.Path, pool.VolumeID, pool.volumeRelative, pool.Filesystem, pool.LastActivityAt)
	return pool, err
}

// AddRemotePool 註冊唯讀 WebDAV 池；目前只支援 index-only discovery。
func (s *Store) AddRemotePool(name, remoteURL, username, password, root, tlsCA string, tlsSkipVerify bool) (Pool, error) {
	name = strings.TrimSpace(name)
	remoteURL = strings.TrimSpace(remoteURL)
	tlsCA = strings.TrimSpace(tlsCA)
	if tlsSkipVerify && tlsCA != "" {
		return Pool{}, fmt.Errorf("tlsSkipVerify 與自訂 CA 不可同時設定")
	}
	if len(tlsCA) > 64<<10 {
		return Pool{}, fmt.Errorf("自訂 CA 超過 64 KiB")
	}
	if tlsCA != "" {
		certs := x509.NewCertPool()
		if !certs.AppendCertsFromPEM([]byte(tlsCA)) {
			return Pool{}, fmt.Errorf("自訂 CA 不是有效的 PEM 憑證")
		}
	}
	if name == "" || remoteURL == "" {
		return Pool{}, fmt.Errorf("請提供名稱與 WebDAV URL")
	}
	parsed, err := url.Parse(remoteURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Pool{}, fmt.Errorf("WebDAV URL 必須是 http 或 https")
	}
	pool := Pool{
		ID: identifier(), Name: name, Path: remoteURL, Online: true, Enabled: true,
		Provider: "webdav", RemoteURL: remoteURL, RemoteUser: username, RemoteRoot: strings.TrimSpace(root), TLSSkipVerify: tlsSkipVerify, TLSCASet: strings.TrimSpace(tlsCA) != "", TLSCA: strings.TrimSpace(tlsCA),
		Filesystem: "WebDAV", VolumeStatus: "unsupported", LastActivityAt: timestamp(),
		ScanExcludeSystem: true, IndexOnly: true, WholeDisk: false,
	}
	_, err = s.db.Exec("INSERT INTO pools(id,name,path,volume_id,volume_relative,filesystem,enabled,last_activity_at,scan_exclude_system,index_only,provider,remote_url,remote_user,remote_root,tls_skip_verify,tls_ca) VALUES(?,?,?,?,?,?,1,?,1,1,'webdav',?,?,?,?,?)", pool.ID, pool.Name, pool.Path, pool.VolumeID, pool.volumeRelative, pool.Filesystem, pool.LastActivityAt, pool.RemoteURL, pool.RemoteUser, pool.RemoteRoot, pool.TLSSkipVerify, pool.TLSCA)
	if err != nil {
		return Pool{}, err
	}
	if err := s.saveWebDAVCredentials(pool.ID, username, password); err != nil {
		_, _ = s.db.Exec("DELETE FROM pools WHERE id=?", pool.ID)
		return Pool{}, err
	}
	return pool, nil
}

// SetPoolEnabled 切換儲存池的掃描、匯入與播放可用性；影片索引與描述保留。
func (s *Store) SetPoolEnabled(id string, enabled bool) (Pool, error) {
	result, err := s.db.Exec("UPDATE pools SET enabled=? WHERE id=?", enabled, id)
	if err != nil {
		return Pool{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Pool{}, err
	}
	if affected == 0 {
		return Pool{}, sql.ErrNoRows
	}
	return s.poolByID(id)
}

func isWholeDisk(path string) bool {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	if volume == "" {
		return clean == string(filepath.Separator)
	}
	return clean == volume+string(filepath.Separator)
}

func (s *Store) touchPoolActivity(id string, at time.Time) error {
	_, err := s.db.Exec("UPDATE pools SET last_activity_at=? WHERE id=?", at.UTC().Format(time.RFC3339Nano), id)
	return err
}

// SetPoolIndexOnly 切換只索引模式；既有 discovered_media 會保留。
func (s *Store) SetPoolIndexOnly(id string, indexOnly bool) (Pool, error) {
	result, err := s.db.Exec("UPDATE pools SET index_only=? WHERE id=?", indexOnly, id)
	if err != nil {
		return Pool{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Pool{}, err
	}
	if affected == 0 {
		return Pool{}, sql.ErrNoRows
	}
	return s.poolByID(id)
}

// SetPoolScanExcludeSystem 控制安全掃描預設；關閉代表使用者明確選擇全盤掃描。
func (s *Store) SetPoolScanExcludeSystem(id string, exclude bool) (Pool, error) {
	result, err := s.db.Exec("UPDATE pools SET scan_exclude_system=? WHERE id=?", exclude, id)
	if err != nil {
		return Pool{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Pool{}, err
	}
	if affected == 0 {
		return Pool{}, sql.ErrNoRows
	}
	return s.poolByID(id)
}

func (s *Store) poolByID(id string) (Pool, error) {
	var pool Pool
	var enabled, excludeSystem, indexOnly int
	err := s.db.QueryRow("SELECT id,name,path,volume_id,volume_relative,filesystem,enabled,last_activity_at,scan_exclude_system,index_only,provider,remote_url,remote_user,remote_root FROM pools WHERE id=?", id).
		Scan(&pool.ID, &pool.Name, &pool.Path, &pool.VolumeID, &pool.volumeRelative, &pool.Filesystem, &enabled, &pool.LastActivityAt, &excludeSystem, &indexOnly, &pool.Provider, &pool.RemoteURL, &pool.RemoteUser, &pool.RemoteRoot)
	if err != nil {
		return Pool{}, err
	}
	pool.Enabled = enabled != 0
	pool.TLSCASet = pool.TLSCA != ""
	pool.ScanExcludeSystem = excludeSystem != 0
	pool.IndexOnly = indexOnly != 0
	if pool.Provider == "webdav" {
		pool.Online = true
		pool.Filesystem = "WebDAV"
		pool.VolumeStatus = "unsupported"
		pool.WholeDisk = false
	} else {
		pool.WholeDisk = isWholeDisk(pool.Path)
		applyVolumeStatus(&pool)
	}
	if effective, err := effectivePoolPath(pool.Path, pool.VolumeID, pool.volumeRelative); err == nil {
		if info, statErr := os.Stat(effective); statErr == nil && info.IsDir() {
			pool.Online = true
		}
	}
	return pool, nil
}

// RefreshPoolMetadata 重新查詢在線池的檔案系統與 volume 身分；離線池保留舊值。
func (s *Store) RefreshPoolMetadata() ([]Pool, error) {
	pools, err := s.Pools()
	if err != nil {
		return nil, err
	}
	for _, pool := range pools {
		root, err := effectivePoolPath(pool.Path, pool.VolumeID, pool.volumeRelative)
		if err != nil {
			continue
		}
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		volumeID, volumeRelative, filesystem := volumeDetailsFor(root)
		if volumeID == "" {
			volumeID, volumeRelative = pool.VolumeID, pool.volumeRelative
		}
		if filesystem == "" {
			filesystem = pool.Filesystem
		}
		if volumeID == pool.VolumeID && volumeRelative == pool.volumeRelative && filesystem == pool.Filesystem {
			continue
		}
		if _, err := s.db.Exec("UPDATE pools SET volume_id=?,volume_relative=?,filesystem=? WHERE id=?", volumeID, volumeRelative, filesystem, pool.ID); err != nil {
			return nil, err
		}
	}
	return s.Pools()
}

func applyVolumeStatus(pool *Pool) {
	switch {
	case pool.VolumeID != "":
		pool.VolumeStatus = "tracked"
	case !volumeTrackingSupported():
		pool.VolumeStatus = "unsupported"
	default:
		pool.VolumeStatus = "unbound"
	}
}

// effectivePoolPath 優先使用 volume GUID；一旦記錄了 volume ID 就不回退到可能被其他磁碟佔用的盤符。
func effectivePoolPath(path, volumeID, relative string) (string, error) {
	if volumeID == "" {
		return path, nil
	}
	return resolveVolumePath(volumeID, relative)
}

// backfillPoolMetadata 為舊資料補上 volume GUID/檔案系統；路徑離線時先跳過，下次啟動再試。
func (s *Store) backfillPoolMetadata() error {
	rows, err := s.db.Query("SELECT id,path,volume_id,volume_relative,filesystem FROM pools WHERE volume_id='' OR filesystem=''")
	if err != nil {
		return err
	}
	type item struct {
		id             string
		path           string
		volumeID       string
		volumeRelative string
		filesystem     string
	}
	items := []item{}
	for rows.Next() {
		var current item
		if err := rows.Scan(&current.id, &current.path, &current.volumeID, &current.volumeRelative, &current.filesystem); err != nil {
			rows.Close()
			return err
		}
		items = append(items, current)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, current := range items {
		volumeID, volumeRelative, filesystem := volumeDetailsFor(current.path)
		if volumeID == "" {
			volumeID, volumeRelative = current.volumeID, current.volumeRelative
		}
		if filesystem == "" {
			filesystem = current.filesystem
		}
		if volumeID == current.volumeID && volumeRelative == current.volumeRelative && filesystem == current.filesystem {
			continue
		}
		if _, err := s.db.Exec("UPDATE pools SET volume_id=?,volume_relative=?,filesystem=? WHERE id=?", volumeID, volumeRelative, filesystem, current.id); err != nil {
			return err
		}
	}
	return nil
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
