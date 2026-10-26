package library

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Metadata 是發布者提供的影片描述；空白授權代表未提供，不能推導轉載權利。
// Adult 僅標記內容，不代表已進行年齡驗證或存取限制。
type Metadata struct {
	Description    string    `json:"description"`
	Category       string    `json:"category"`
	Adult          bool      `json:"adult"`
	License        string    `json:"license"`
	Tags           []string  `json:"tags"`
	Creators       []Creator `json:"creators"`
	WorkType       string    `json:"workType"`
	SourceURL      string    `json:"sourceUrl"`
	SourcePlatform string    `json:"sourcePlatform"`
	SourcePending  bool      `json:"sourcePending"`
	RepostNote     string    `json:"repostNote"`
	Language       string    `json:"language"`
}

// Creator 描述原作者，而非匯入者；URL 可指向作者個人頁或頻道。
type Creator struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (m Metadata) normalize() (Metadata, error) {
	m.Description = strings.TrimSpace(m.Description)
	m.Category = strings.TrimSpace(m.Category)
	m.License = strings.TrimSpace(m.License)
	m.WorkType = strings.TrimSpace(m.WorkType)
	if m.WorkType == "" {
		m.WorkType = "unknown"
	}
	if m.WorkType != "unknown" && m.WorkType != "original" && m.WorkType != "repost" {
		return m, fmt.Errorf("作品類型無效")
	}
	m.SourceURL = strings.TrimSpace(m.SourceURL)
	m.SourcePlatform = strings.TrimSpace(m.SourcePlatform)
	m.RepostNote = strings.TrimSpace(m.RepostNote)
	m.Language = strings.TrimSpace(m.Language)
	if utf8.RuneCountInString(m.SourcePlatform) > 80 || utf8.RuneCountInString(m.RepostNote) > 2000 || utf8.RuneCountInString(m.Language) > 80 {
		return m, fmt.Errorf("來源平台、轉載說明或語言超過長度上限")
	}
	if err := validateURL(m.SourceURL); err != nil {
		return m, err
	}
	if m.SourceURL != "" {
		m.SourcePending = false
	}
	if len(m.Creators) > 20 {
		return m, fmt.Errorf("最多填寫 20 位原作者")
	}
	creators := make([]Creator, 0, len(m.Creators))
	for _, creator := range m.Creators {
		creator.Name = strings.TrimSpace(creator.Name)
		creator.URL = strings.TrimSpace(creator.URL)
		if creator.Name == "" || utf8.RuneCountInString(creator.Name) > 120 {
			return m, fmt.Errorf("原作者名稱必填且不可超過 120 字")
		}
		if err := validateURL(creator.URL); err != nil {
			return m, err
		}
		creators = append(creators, creator)
	}
	m.Creators = creators
	if m.WorkType == "repost" && (len(m.Creators) == 0 || (m.SourceURL == "" && !m.SourcePending)) {
		return m, fmt.Errorf("轉載需提供原作者及原始作品連結，或明確勾選來源待補")
	}
	tags := make([]string, 0, len(m.Tags))
	seen := map[string]bool{}
	for _, tag := range m.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > 40 {
			return m, fmt.Errorf("每個 Tag 最多 40 字")
		}
		key := strings.ToLower(tag)
		if !seen[key] {
			tags = append(tags, tag)
			seen[key] = true
		}
	}
	if len(tags) > 20 {
		return m, fmt.Errorf("每部影片最多 20 個 Tag")
	}
	m.Tags = tags
	if utf8.RuneCountInString(m.Description) > 10000 || utf8.RuneCountInString(m.Category) > 80 || utf8.RuneCountInString(m.License) > 500 {
		return m, fmt.Errorf("內容簡介、分類或授權資訊超過長度上限")
	}
	return m, nil
}

func validateURL(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if len(value) > 2048 || err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("連結必須是有效的 HTTP 或 HTTPS 網址，且不可包含帳號密碼")
	}
	return nil
}
