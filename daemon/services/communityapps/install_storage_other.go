//go:build !linux

package communityapps

import (
	"os"

	"golang.org/x/sys/unix"
)

func mkdirInstallDirectory(parent *os.File, name string, native bool) error {
	if native {
		return ErrStorageState
	}
	return unix.Mkdirat(int(parent.Fd()), name, 0o777)
}
