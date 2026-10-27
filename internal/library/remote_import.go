package library

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"
	"time"
)

// ImportRemoteVideo 建立遠端 library 參考；不下载檔案本體。
func (s *Store) ImportRemoteVideo(ctx context.Context, poolID, relative, title string, details Metadata) (Video, error) {
	var result Video
	pool, err := s.poolByID(poolID)
	if err != nil {
		return result, err
	}
	if pool.Provider != "webdav" {
		return result, fmt.Errorf("此儲存池不是 WebDAV")
	}
	if !pool.Enabled {
		return result, ErrPoolDisabled
	}
	if !safeRemoteRelative(relative) {
		return result, fmt.Errorf("影片路徑無效")
	}
	item, err := s.discoveredItem(ctx, poolID, relative)
	if err != nil {
		return result, err
	}
	details, err = details.normalize()
	if err != nil {
		return result, err
	}
	hash := s.openListSHA256(ctx, pool, relative)
	hashStatus := "unverified"
	if hash != "" {
		hashStatus = "verified"
	}
	if strings.TrimSpace(title) == "" {
		title = pathpkg.Base(relative)
	}
	result = Video{
		ID: identifier(), PublicationID: identifier(), ReplicaID: identifier(), PoolID: poolID,
		Title: strings.TrimSpace(title), FileHash: hash, SourceHash: hash, Size: item.Size,
		ImportedAt: time.Now().UTC().Format(time.RFC3339Nano), Available: true,
		Provider: "webdav", HashStatus: hashStatus, relativePath: relative,
		modifiedAt: item.ModifiedAt * int64(time.Millisecond),
	}
	result.Metadata = details
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO videos (`+videoColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, result.ID, result.PublicationID, result.ReplicaID, result.PoolID, result.Title, result.FileHash, result.SourceHash, result.Size, result.ImportedAt, result.relativePath, result.modifiedAt, result.Description, result.Category, result.Adult, result.License, result.Provider, result.HashStatus); err != nil {
		return result, err
	}
	if err := saveAttribution(ctx, tx, result.ID, details); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	s.publish("jobs")
	return result, nil
}

func safeRemoteRelative(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// openListSHA256 嘗試從 OpenList JSON API 取得 driver 已算好的 SHA-256；失敗時回空字串。
func (s *Store) openListSHA256(ctx context.Context, pool Pool, relative string) string {
	base, err := url.Parse(strings.TrimSpace(pool.RemoteURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return ""
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	if !strings.HasSuffix(base.Path, "/dav") {
		return ""
	}
	apiPath := strings.TrimSuffix(base.Path, "/dav") + "/api"
	username, password, err := s.WebDAVCredentials(pool.ID)
	if err != nil {
		return ""
	}
	client := &http.Client{Timeout: 10 * time.Second}
	loginBody, _ := json.Marshal(map[string]string{"username": username, "password": password})
	loginURL := *base
	loginURL.Path = apiPath + "/auth/login"
	loginRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL.String(), bytes.NewReader(loginBody))
	if err != nil {
		return ""
	}
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse, err := client.Do(loginRequest)
	if err != nil {
		return ""
	}
	defer loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		return ""
	}
	var login struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(loginResponse.Body, 64<<10)).Decode(&login); err != nil || login.Data.Token == "" {
		return ""
	}
	remotePath := pathpkg.Join("/", strings.Trim(pool.RemoteRoot, "/"), relative)
	if remotePath == "/" || remotePath == "" {
		return ""
	}
	getURL := *base
	getURL.Path = apiPath + "/fs/get"
	query := url.Values{"path": []string{remotePath}}
	getURL.RawQuery = query.Encode()
	getRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL.String(), nil)
	if err != nil {
		return ""
	}
	getRequest.Header.Set("Authorization", login.Data.Token)
	getResponse, err := client.Do(getRequest)
	if err != nil {
		return ""
	}
	defer getResponse.Body.Close()
	if getResponse.StatusCode != http.StatusOK {
		return ""
	}
	var value struct {
		Data struct {
			HashInfo map[string]string `json:"hash_info"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(getResponse.Body, 1<<20)).Decode(&value); err != nil {
		return ""
	}
	for name, candidate := range value.Data.HashInfo {
		if !strings.EqualFold(name, "sha256") {
			continue
		}
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		decoded, err := hex.DecodeString(candidate)
		if err == nil && len(decoded) == 32 {
			return candidate
		}
	}
	return ""
}
