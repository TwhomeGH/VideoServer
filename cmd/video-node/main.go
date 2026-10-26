// Video Node 是可獨立運作的影片節點入口。
package main

import (
	"github.com/TwhomeGH/VideoServer/internal/runtime"
	"os"
)

func main() { os.Exit(runtime.Main("video-node", 18610)) }
