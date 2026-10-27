// Package protocol 提供 DVN 跨服務的 HTTP JSON 協定版本與能力公告。
package protocol

// Current 是目前服務實作的通訊協定版本。
const Current = 1

// MinSupported 與 MaxSupported 是伺服器可接受的協定版本範圍；目前只有 v1。
const (
	MinSupported = 1
	MaxSupported = 1
)

// Header 是請求攜帶的協定版本標頭。
const Header = "X-DVN-Protocol-Version"

// Capability 是帶版本的功能公告；同一名稱可隨時間增加版本。
type Capability struct {
	Name     string `json:"name"`
	Versions []int  `json:"versions"`
}

var commonCapabilities = []Capability{
	{Name: "auth.session", Versions: []int{1}},
	{Name: "config.overrides", Versions: []int{1}},
	{Name: "system.restart", Versions: []int{1}},
	{Name: "monitor.resources", Versions: []int{1}},
	{Name: "openapi", Versions: []int{1}},
}

var videoCapabilities = []Capability{
	{Name: "library.query", Versions: []int{1}},
	{Name: "library.scan", Versions: []int{1}},
	{Name: "video.range", Versions: []int{1}},
	{Name: "video.probe", Versions: []int{1}},
	{Name: "jobs.import", Versions: []int{1}},
}

// ServiceType 回傳對外協商使用的服務類型；目前仍是三服務，Image／Post 會再擴充。
func ServiceType(service string) string {
	switch service {
	case "video-node", "central", "transcode-worker":
		return service
	default:
		return ""
	}
}

// ContentTypes 公告此節點可提供的內容類型；Central／Worker 目前沒有內容庫。
func ContentTypes(service string) []string {
	switch service {
	case "video-node":
		return []string{"video"}
	default:
		return []string{}
	}
}

// SupportedVersions 回傳目前伺服器支援的完整版本清單，供公告與錯誤回應使用。
func SupportedVersions() []int {
	versions := make([]int, 0, MaxSupported-MinSupported+1)
	for version := MinSupported; version <= MaxSupported; version++ {
		versions = append(versions, version)
	}
	return versions
}

// Negotiate 回傳 client 與伺服器版本範圍的交集最高版本。
func Negotiate(clientMin, clientMax int) (int, bool) {
	if clientMin < MinSupported || clientMax < clientMin {
		return 0, false
	}
	low := clientMin
	if low < MinSupported {
		low = MinSupported
	}
	high := clientMax
	if high > MaxSupported {
		high = MaxSupported
	}
	if low > high {
		return 0, false
	}
	return high, true
}

// Compatible 回傳 peerVersion 是否在伺服器支援範圍內。
func Compatible(peerVersion int) bool {
	return peerVersion >= MinSupported && peerVersion <= MaxSupported
}

// CapabilityList 回傳 service 的能力與版本；未知服務回傳空清單。
func CapabilityList(service string) []Capability {
	if service != "video-node" && service != "central" && service != "transcode-worker" {
		return []Capability{}
	}
	features := append([]Capability(nil), commonCapabilities...)
	if service == "video-node" {
		features = append(features, videoCapabilities...)
	}
	return features
}

// Capabilities 回傳向後相容的能力名稱清單，維持既有欄位與順序。
func Capabilities(service string) []string {
	list := CapabilityList(service)
	features := make([]string, 0, len(list))
	for _, capability := range list {
		features = append(features, capability.Name)
	}
	return features
}

// CapabilityVersions 回傳名稱到版本清單的映射，供新版 client 協商。
func CapabilityVersions(service string) map[string][]int {
	list := CapabilityList(service)
	versions := make(map[string][]int, len(list))
	for _, capability := range list {
		versions[capability.Name] = append([]int(nil), capability.Versions...)
	}
	return versions
}
