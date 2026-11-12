//go:build linux

package plugininventory

import (
	"os"
	"syscall"
	"time"
)

func fileChangeTime(info os.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)
	}
	return info.ModTime()
}
