package library

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const webDAVTimeout = 30 * time.Second
const webDAVMaxDepth = 64

type webDAVEntry struct {
	Relative string
	Size     int64
	Modified time.Time
}

type webDAVMultistatus struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat []struct {
			Prop struct {
				ResourceType struct {
					Collection []struct{} `xml:"collection"`
				} `xml:"resourcetype"`
				ContentLength string `xml:"getcontentlength"`
				LastModified  string `xml:"getlastmodified"`
			} `xml:"prop"`
			Status string `xml:"status"`
		} `xml:"propstat"`
	} `xml:"response"`
}

// webDAVTarget 組合 WebDAV base、root 與 discovered 相對路徑；relative 需先驗證。
func webDAVTarget(pool Pool, relative string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(pool.RemoteURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("WebDAV URL 無效")
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	if root := strings.Trim(strings.TrimSpace(pool.RemoteRoot), "/"); root != "" {
		base.Path = path.Join(base.Path, root)
	}
	base.Path = path.Join(base.Path, relative)
	return base, nil
}

// listWebDAV 以 Depth:1 遞迴列出遠端影片；不讀取檔案內容。
func (s *Store) listWebDAV(ctx context.Context, pool Pool) ([]webDAVEntry, error) {
	username, password, err := s.WebDAVCredentials(pool.ID)
	if err != nil {
		return nil, fmt.Errorf("讀取 WebDAV 憑證失敗：%w", err)
	}
	base, err := url.Parse(strings.TrimSpace(pool.RemoteURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("WebDAV URL 無效")
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	rootPath := base.Path
	if root := strings.Trim(strings.TrimSpace(pool.RemoteRoot), "/"); root != "" {
		rootPath = path.Join(rootPath, root)
	}
	if rootPath == "" {
		rootPath = "/"
	}
	client := &http.Client{Timeout: webDAVTimeout}
	entries := []webDAVEntry{}
	seen := map[string]bool{}
	if err := s.walkWebDAV(ctx, client, base, username, password, rootPath, "", 0, seen, &entries, pool); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) walkWebDAV(ctx context.Context, client *http.Client, base *url.URL, username, password, rootPath, relative string, depth int, seen map[string]bool, entries *[]webDAVEntry, pool Pool) error {
	if depth > webDAVMaxDepth {
		return fmt.Errorf("WebDAV 目錄深度超過 %d", webDAVMaxDepth)
	}
	currentPath := rootPath
	if relative != "" {
		currentPath = path.Join(rootPath, strings.TrimPrefix(relative, "/"))
	}
	if seen[currentPath] {
		return nil
	}
	seen[currentPath] = true
	target := *base
	target.Path = currentPath
	request, err := http.NewRequestWithContext(ctx, "PROPFIND", target.String(), nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth(username, password)
	request.Header.Set("Depth", "1")
	request.Header.Set("Content-Type", "application/xml; charset=utf-8")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMultiStatus {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("WebDAV PROPFIND %s：%s %s", target.String(), response.Status, strings.TrimSpace(string(body)))
	}
	var document webDAVMultistatus
	if err := xml.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&document); err != nil {
		return err
	}
	for _, item := range document.Responses {
		entryPath := item.Href
		parsed, err := url.Parse(entryPath)
		if err == nil {
			entryPath = parsed.Path
		}
		decoded, err := url.PathUnescape(entryPath)
		if err != nil {
			continue
		}
		entryPath = strings.TrimSuffix(decoded, "/")
		if entryPath == strings.TrimSuffix(rootPath, "/") {
			continue
		}
		if !strings.HasPrefix(entryPath, strings.TrimSuffix(rootPath, "/")+"/") {
			continue
		}
		itemRelative := strings.TrimPrefix(entryPath, strings.TrimSuffix(rootPath, "/")+"/")
		if itemRelative == "" {
			continue
		}
		if len(item.Propstat) == 0 {
			continue
		}
		prop := item.Propstat[0].Prop
		isDirectory := len(prop.ResourceType.Collection) > 0
		if isDirectory {
			if pool.ScanExcludeSystem && shouldExcludeSystemPath(itemRelative) {
				continue
			}
			if err := s.walkWebDAV(ctx, client, base, username, password, rootPath, itemRelative, depth+1, seen, entries, pool); err != nil {
				return err
			}
			continue
		}
		if !s.supportedVideo(itemRelative) {
			continue
		}
		size := int64(0)
		if prop.ContentLength != "" {
			size, _ = strconv.ParseInt(prop.ContentLength, 10, 64)
		}
		modified, _ := http.ParseTime(prop.LastModified)
		*entries = append(*entries, webDAVEntry{Relative: itemRelative, Size: size, Modified: modified})
	}
	return nil
}
