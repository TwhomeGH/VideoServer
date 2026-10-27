// Package buildinfo 提供服務的建置識別。
package buildinfo

import (
	"os"
	"runtime/debug"

	"github.com/TwhomeGH/VideoServer/pkg/protocol"
)

// Version、BuildTime、Channel 可由建置時的 -ldflags 獨立注入。
var (
	Version   = "0.1.0"
	BuildTime = "unknown"
	Channel   = "development"
	// Commit 與 Dirty 供不含 .git 的容器建置明確注入；空值時採 Go VCS 資訊。
	Commit = ""
	Dirty  = ""
)

// Identity 是健康檢查與診斷使用的版本快照。
type Identity struct {
	Service           string `json:"service"`
	Version           string `json:"version"`
	Commit            string `json:"commit"`
	BuildTime         string `json:"buildTime"`
	Channel           string `json:"channel"`
	Dirty             bool   `json:"dirty"`
	ProtocolVersion   int    `json:"protocolVersion"`
	ImageBuildTime    string `json:"imageBuildTime"`
	ImageSourceDigest string `json:"imageSourceDigest"`
}

// Current 回傳指定 service 的識別；go run 無 VCS 資訊時明確標示 unknown。
func Current(service string) Identity {
	i := Identity{
		Service: service, Version: Version, Commit: "unknown", BuildTime: BuildTime, Channel: Channel, ProtocolVersion: protocol.Current,
		ImageBuildTime: envOr("DVN_IMAGE_BUILT_AT"), ImageSourceDigest: envOr("DVN_IMAGE_SOURCE_DIGEST"),
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				i.Commit = setting.Value
			case "vcs.modified":
				i.Dirty = setting.Value == "true"
			}
		}
	}
	if Commit != "" {
		i.Commit = Commit
	}
	if Dirty != "" {
		i.Dirty = Dirty == "true"
	}
	return i
}

// envOr 讀取 image 建置工具注入的環境變數；未提供時明確回傳 unknown。
func envOr(key string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return "unknown"
}
