package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Thumbnail 從本機或遠端影片擷取一張 JPEG 封面。
func Thumbnail(ctx context.Context, target, headers, caFile string, tlsSkipVerify bool, executable, output string, offset, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"-v", "error", "-protocol_whitelist", "file,http,https,tcp,tls"}
	if offset > 0 {
		args = append(args, "-ss", strconv.FormatFloat(offset.Seconds(), 'f', 3, 64))
	}
	if headers != "" {
		args = append(args, "-headers", headers)
	}
	if tlsSkipVerify {
		args = append(args, "-tls_verify", "0")
	} else if caFile != "" {
		args = append(args, "-ca_file", caFile)
	}
	args = append(args, "-i", target, "-frames:v", "1", "-an", "-vf", "scale=480:-2", "-q:v", "3", "-y", output)
	command := exec.CommandContext(ctx, executable, args...)
	diagnostics := &limitedBuffer{limit: 8192}
	command.Stderr = diagnostics
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("封面擷取逾時：%w", ctx.Err())
		}
		if _, ok := err.(*exec.Error); ok {
			return fmt.Errorf("找不到 FFmpeg，請確認 ffmpegPath")
		}
		detail := strings.TrimSpace(diagnostics.String())
		if len(detail) > 1000 {
			detail = detail[:1000]
		}
		return fmt.Errorf("FFmpeg 封面擷取失敗：%s", detail)
	}
	if info, err := os.Stat(output); err != nil || info.Size() == 0 {
		return fmt.Errorf("FFmpeg 未產生封面")
	}
	return nil
}
