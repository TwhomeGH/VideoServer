// Central 是索引服務的入口，目前提供啟動與診斷骨架。
package main

import (
	"github.com/TwhomeGH/VideoServer/internal/runtime"
	"os"
)

func main() { os.Exit(runtime.Main("central", 18620)) }
