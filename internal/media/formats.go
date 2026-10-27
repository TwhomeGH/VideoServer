package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// 預設只放常見的影片容器；.ts／.mts 與 TypeScript 衝突，需由 extra 明確開啟。
var defaultVideoExtensions = []string{
	".mp4", ".webm", ".mkv", ".mov", ".avi", ".m4v",
	".mpg", ".mpeg", ".wmv", ".flv", ".3gp", ".ogv",
	".m2ts", ".vob", ".mxf", ".f4v", ".asf", ".dv",
}

var extensionDemuxer = map[string]string{
	".mp4": "mov", ".m4v": "mov", ".mov": "mov", ".3gp": "mov", ".f4v": "mov",
	".mkv": "matroska", ".webm": "matroska",
	".avi": "avi", ".mpg": "mpeg", ".mpeg": "mpeg", ".vob": "mpeg",
	".wmv": "asf", ".asf": "asf", ".flv": "flv", ".ogv": "ogg",
	".m2ts": "mpegts", ".ts": "mpegts", ".mts": "mpegts",
	".mxf": "mxf", ".dv": "dv",
}

// VideoFormats 是掃描前使用的影片容器 allowlist，並以 FFprobe 的 demuxer 清單驗證可用性。
type VideoFormats struct {
	extensions map[string]struct{}
	warnings   []string
}

// LoadVideoFormats 查詢 ffprobe -formats，將 extra 與預設清單交集到實際支援的 demuxer。
// 查詢失敗時仍保留預設清單，讓掃描可用；實際檔案再由 FFprobe 驗證。
func LoadVideoFormats(ctx context.Context, ffprobePath string, extra []string) *VideoFormats {
	candidates := append([]string{}, defaultVideoExtensions...)
	candidates = append(candidates, extra...)
	available, err := queryDemuxers(ctx, ffprobePath)
	formats := &VideoFormats{extensions: map[string]struct{}{}}
	if err != nil {
		formats.warnings = append(formats.warnings, "無法查詢 FFprobe demuxer，使用內建影片格式清單："+err.Error())
		for _, extension := range candidates {
			if normalized := normalizeExtension(extension); normalized != "" {
				formats.extensions[normalized] = struct{}{}
			}
		}
		return formats
	}
	seen := map[string]bool{}
	for _, extension := range candidates {
		normalized := normalizeExtension(extension)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		demuxer := extensionDemuxer[normalized]
		if demuxer != "" && !available[demuxer] {
			formats.warnings = append(formats.warnings, fmt.Sprintf("FFprobe 不支援 %s 所需的 %s demuxer，已略過", normalized, demuxer))
			continue
		}
		formats.extensions[normalized] = struct{}{}
	}
	return formats
}

// DefaultVideoFormats 供不查詢 FFprobe 的測試與降級路徑使用。
func DefaultVideoFormats() *VideoFormats {
	formats := &VideoFormats{extensions: map[string]struct{}{}}
	for _, extension := range defaultVideoExtensions {
		formats.extensions[normalizeExtension(extension)] = struct{}{}
	}
	return formats
}

func (f *VideoFormats) Warnings() []string {
	if f == nil {
		return nil
	}
	return append([]string(nil), f.warnings...)
}

func (f *VideoFormats) Extensions() []string {
	if f == nil {
		return nil
	}
	result := make([]string, 0, len(f.extensions))
	for extension := range f.extensions {
		result = append(result, extension)
	}
	return result
}

// Supports 判斷路徑是否是可掃描的影片；不明的 .ts／.mts 會再做 MPEG-TS 封包檢查。
func (f *VideoFormats) Supports(path string) bool {
	if f == nil || len(f.extensions) == 0 {
		return false
	}
	extension := normalizeExtension(filepath.Ext(path))
	if _, ok := f.extensions[extension]; !ok {
		return false
	}
	switch extension {
	case ".ts", ".mts", ".m2ts":
		return looksLikeTransportStream(path, extension == ".m2ts" || extension == ".mts")
	default:
		return true
	}
}

func normalizeExtension(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, ".") {
		value = "." + value
	}
	if strings.ContainsAny(value, `/\`) || len(value) > 12 {
		return ""
	}
	return value
}

func queryDemuxers(ctx context.Context, ffprobePath string) (map[string]bool, error) {
	if strings.TrimSpace(ffprobePath) == "" {
		return nil, fmt.Errorf("ffprobe 路徑為空")
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(queryCtx, ffprobePath, "-hide_banner", "-formats").Output()
	if err != nil {
		return nil, err
	}
	return parseDemuxers(string(output)), nil
}

func parseDemuxers(output string) map[string]bool {
	result := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.Contains(fields[0], "D") {
			continue
		}
		for _, name := range strings.Split(fields[1], ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				result[name] = true
			}
		}
	}
	return result
}

// looksLikeTransportStream 要求多個連續同步位元組，避免把 TypeScript 原始碼當成 MPEG-TS。
func looksLikeTransportStream(path string, m2ts bool) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	buffer := make([]byte, 2048)
	n, err := file.Read(buffer)
	if err != nil && n == 0 {
		return false
	}
	offset, stride := 0, 188
	if m2ts {
		offset, stride = 4, 192
	}
	if offset+stride*2 >= n {
		return false
	}
	return buffer[offset] == 0x47 && buffer[offset+stride] == 0x47 && buffer[offset+stride*2] == 0x47
}
