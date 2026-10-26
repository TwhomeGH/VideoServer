//go:build !windows && !linux

package diagnostics

import (
	"fmt"
	"time"
)

func processCPU() (time.Duration, error) {
	return 0, fmt.Errorf("此平台尚未提供程序 CPU 取樣")
}
