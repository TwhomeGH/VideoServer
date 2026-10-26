package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/media"
)

const videoColumns = `id,publication_id,replica_id,pool_id,title,file_hash,source_hash,size,imported_at,relative_path,modified_at,description,category,adult,license`

func scanVideo(row interface{ Scan(...any) error }) (Video, error) {
	var v Video
	err := row.Scan(&v.ID, &v.PublicationID, &v.ReplicaID, &v.PoolID, &v.Title, &v.FileHash, &v.SourceHash, &v.Size, &v.ImportedAt, &v.relativePath, &v.modifiedAt, &v.Description, &v.Category, &v.Adult, &v.License)
	return v, err
}

func (s *Store) poolPath(id string) (string, error) {
	var path string
	err := s.db.QueryRow("SELECT path FROM pools WHERE id=?", id).Scan(&path)
	return path, err
}

// Import 對 poolID 內的既有影片建立索引並計算 SHA-256，不複製檔案。
// 這是同步索引底層；正式管理 API 與掃描透過 ImportQueue 先進行 FFprobe 檢查。
// ctx 取消或來源讀取期間變動時不提交索引；同一路徑重複匯入會失敗。
// metadata 可省略或提供一份描述資料，長度超限會在讀取影片前回傳錯誤。
func (s *Store) Import(ctx context.Context, poolID, relative, title string, metadata ...Metadata) (Video, error) {
	if len(metadata) > 1 {
		return Video{}, fmt.Errorf("每次匯入僅能提供一份描述資料")
	}
	var details Metadata
	if len(metadata) == 1 {
		details = metadata[0]
	}
	return s.importVideo(ctx, poolID, relative, title, details, nil)
}

type importOptions struct {
	media    *media.Info
	jobID    string
	expected os.FileInfo
}

func (s *Store) importVideo(ctx context.Context, poolID, relative, title string, details Metadata, options *importOptions) (Video, error) {
	var v Video
	details, err := details.normalize()
	if err != nil {
		return v, err
	}
	root, err := s.poolPath(poolID)
	if err != nil {
		return v, err
	}
	path, err := resolve(root, relative)
	if err != nil {
		return v, err
	}
	if !supportedVideo(path) {
		return v, fmt.Errorf("不支援的影片副檔名")
	}
	file, err := os.Open(path)
	if err != nil {
		return v, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return v, err
	}
	if !before.Mode().IsRegular() {
		return v, fmt.Errorf("來源不是一般檔案")
	}
	if options != nil && options.expected != nil && (!os.SameFile(before, options.expected) || before.Size() != options.expected.Size() || !before.ModTime().Equal(options.expected.ModTime())) {
		return v, fmt.Errorf("來源已變更，請重新匯入")
	}
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	var processed int64
	lastProgress := time.Time{}
	for {
		if err := ctx.Err(); err != nil {
			return v, err
		}
		n, err := file.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
			processed += int64(n)
			if options != nil && options.jobID != "" && (time.Since(lastProgress) > 250*time.Millisecond || processed == before.Size()) {
				if _, err := s.db.ExecContext(ctx, "UPDATE import_jobs SET bytes_processed=?,updated_at=? WHERE id=? AND status='running'", processed, timestamp(), options.jobID); err != nil {
					return v, err
				}
				lastProgress = time.Now()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return v, err
		}
	}
	after, err := file.Stat()
	if err != nil {
		return v, err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return v, fmt.Errorf("來源檔案已變動，請重試")
	}
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(before, current) || current.Size() != before.Size() || !before.ModTime().Equal(current.ModTime()) {
		return v, fmt.Errorf("來源路徑已變更，請重新匯入")
	}
	if strings.TrimSpace(title) == "" {
		title = filepath.Base(path)
	}
	relative, err = filepath.Rel(root, path)
	if err != nil {
		return v, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	v = Video{ID: identifier(), PublicationID: identifier(), ReplicaID: identifier(), PoolID: poolID, Title: strings.TrimSpace(title), FileHash: digest, SourceHash: digest, Size: before.Size(), ImportedAt: time.Now().UTC().Format(time.RFC3339Nano), Available: true, relativePath: relative, modifiedAt: before.ModTime().UnixNano()}
	v.Metadata = details
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO videos (`+videoColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.PublicationID, v.ReplicaID, v.PoolID, v.Title, v.FileHash, v.SourceHash, v.Size, v.ImportedAt, v.relativePath, v.modifiedAt, v.Description, v.Category, v.Adult, v.License)
	if err != nil {
		return v, err
	}
	if err := saveAttribution(ctx, tx, v.ID, details); err != nil {
		return v, err
	}
	if options != nil {
		if options.media != nil {
			payload, err := json.Marshal(options.media)
			if err != nil {
				return v, err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO video_media VALUES(?,?)", v.ID, string(payload)); err != nil {
				return v, err
			}
			v.Media = options.media
		}
		if options.jobID != "" {
			result, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status='completed',phase='completed',video_id=?,bytes_processed=?,total_bytes=?,error='',updated_at=? WHERE id=? AND status='running'`, v.ID, v.Size, v.Size, timestamp(), options.jobID)
			if err != nil {
				return v, err
			}
			count, err := result.RowsAffected()
			if err != nil {
				return v, err
			}
			if count != 1 {
				return v, fmt.Errorf("匯入工作狀態已改變")
			}
		}
	}
	return v, tx.Commit()
}

// Videos 回傳匯入清單及來源可用狀態，來源離線不會移除索引。
func (s *Store) Videos() ([]Video, error) {
	rows, err := s.db.Query("SELECT " + videoColumns + " FROM videos ORDER BY imported_at DESC,id")
	if err != nil {
		return nil, err
	}
	result := []Video{}
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, video)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// 單連線資料庫需先釋放 rows，再查詢各副本，避免互相等待。
	for i := range result {
		if err := s.loadAttribution(&result[i]); err != nil {
			return nil, err
		}
		if err := s.loadMedia(&result[i]); err != nil {
			return nil, err
		}
		file, err := s.openVideo(result[i])
		if err == nil {
			result[i].Available = true
			file.Close()
		}
	}
	return result, nil
}

func (s *Store) openVideo(v Video) (*os.File, error) {
	root, err := s.poolPath(v.PoolID)
	if err != nil {
		return nil, err
	}
	path, err := resolve(root, v.relativePath)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != v.Size || info.ModTime().UnixNano() != v.modifiedAt {
		file.Close()
		return nil, fmt.Errorf("來源離線或已變更，請重新檢查影片")
	}
	return file, nil
}

func (s *Store) video(id string) (Video, error) {
	v, err := scanVideo(s.db.QueryRow("SELECT "+videoColumns+" FROM videos WHERE id=?", id))
	if err == sql.ErrNoRows {
		return v, sql.ErrNoRows
	}
	if err == nil {
		err = s.loadAttribution(&v)
	}
	if err == nil {
		err = s.loadMedia(&v)
	}
	return v, err
}

func (s *Store) loadMedia(video *Video) error {
	var value string
	err := s.db.QueryRow("SELECT details_json FROM video_media WHERE video_id=?", video.ID).Scan(&value)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var info media.Info
	if err := json.Unmarshal([]byte(value), &info); err != nil {
		return err
	}
	video.Media = &info
	return nil
}
