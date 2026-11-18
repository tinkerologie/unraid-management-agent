package communityapps

import "golang.org/x/sys/unix"

func renameExclusive(source, target string) error {
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
}

func renameExclusiveAt(dirFD int, source, target string) error {
	return unix.Renameat2(dirFD, source, dirFD, target, unix.RENAME_NOREPLACE)
}
