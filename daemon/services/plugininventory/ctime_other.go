//go:build !linux

package plugininventory

import (
	"os"
	"time"
)

func fileChangeTime(info os.FileInfo) time.Time {
	return info.ModTime()
}
