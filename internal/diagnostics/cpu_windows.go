package diagnostics

import (
	"golang.org/x/sys/windows"
	"time"
)

func processCPU() (time.Duration, error) {
	process, err := windows.GetCurrentProcess()
	if err != nil {
		return 0, err
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return 0, err
	}
	// GetProcessTimes 的 CPU 時間是 100ns 期間值，不能使用會扣除 epoch 的 Nanoseconds。
	ticks := (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) + (uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))
	return time.Duration(ticks * 100), nil
}
