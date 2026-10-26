// Package media 提供本機影片的 FFprobe 資訊檢查，不負責轉碼。
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Info 保存可顯示與調度使用的技術資訊，不取代發布者描述或內容分級。
type Info struct {
	DurationSeconds float64   `json:"durationSeconds"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	VideoCodec      string    `json:"videoCodec"`
	AudioCodec      string    `json:"audioCodec"`
	Format          string    `json:"format"`
	FrameRate       float64   `json:"frameRate"`
	ProbedAt        time.Time `json:"probedAt"`
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	remaining := b.limit - b.Len()
	if len(value) > remaining {
		if remaining > 0 {
			b.Buffer.Write(value[:remaining])
		}
		return 0, fmt.Errorf("FFprobe 輸出超過上限")
	}
	return b.Buffer.Write(value)
}

// Probe 以 executable 檢查本機 path，timeout 限制外部程序時間。
// 僅允許本機檔案及支援的影片容器，輸出有大小上限；無有效視訊串流時回傳錯誤。
// 此檢查不會完整解碼每一影格，不能保證整部影片無損壞。
func Probe(ctx context.Context, path, executable string, timeout time.Duration) (Info, error) {
	var result Info
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-v", "error", "-protocol_whitelist", "file", "-format_whitelist", "mov,matroska,webm,avi,m4v", "-show_format", "-show_streams", "-of", "json", path)
	output := &limitedBuffer{limit: 1024 * 1024}
	diagnostics := &limitedBuffer{limit: 8192}
	command.Stdout = output
	command.Stderr = diagnostics
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return result, fmt.Errorf("影片檢查已取消或逾時：%w", ctx.Err())
		}
		if _, ok := err.(*exec.Error); ok {
			return result, fmt.Errorf("無法執行 FFprobe，請確認安裝與 ffprobePath：%w", err)
		}
		detail := strings.TrimSpace(diagnostics.String())
		if len(detail) > 1000 {
			detail = detail[:1000]
		}
		return result, fmt.Errorf("FFprobe 無法讀取有效影片：%s", detail)
	}
	return parse(output.Bytes())
}

func parse(data []byte) (Info, error) {
	var input struct {
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			Type        string `json:"codec_type"`
			Codec       string `json:"codec_name"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Duration    string `json:"duration"`
			FrameRate   string `json:"avg_frame_rate"`
			Disposition struct {
				AttachedPicture int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	var info Info
	if err := json.Unmarshal(data, &input); err != nil {
		return info, fmt.Errorf("FFprobe JSON 無效：%w", err)
	}
	info.Format = input.Format.Name
	info.DurationSeconds = number(input.Format.Duration)
	for _, stream := range input.Streams {
		if stream.Type == "audio" && info.AudioCodec == "" {
			info.AudioCodec = stream.Codec
		}
		if stream.Type != "video" || stream.Disposition.AttachedPicture != 0 || info.VideoCodec != "" {
			continue
		}
		if stream.Width <= 0 || stream.Height <= 0 || stream.Codec == "" {
			continue
		}
		info.Width = stream.Width
		info.Height = stream.Height
		info.VideoCodec = stream.Codec
		if info.DurationSeconds == 0 {
			info.DurationSeconds = number(stream.Duration)
		}
		parts := strings.Split(stream.FrameRate, "/")
		if len(parts) == 2 && number(parts[1]) > 0 {
			info.FrameRate = number(parts[0]) / number(parts[1])
		}
	}
	if info.VideoCodec == "" {
		return info, fmt.Errorf("檔案沒有有效的影片視訊串流")
	}
	info.ProbedAt = time.Now().UTC()
	return info, nil
}

func number(value string) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
		return 0
	}
	return parsed
}
