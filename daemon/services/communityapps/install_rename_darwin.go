package communityapps

import "golang.org/x/sys/unix"

func renameExclusive(source, target string) error {
	return unix.RenamexNp(source, target, unix.RENAME_EXCL)
}

func renameExclusiveAt(dirFD int, source, target string) error {
	return unix.RenameatxNp(dirFD, source, dirFD, target, unix.RENAME_EXCL)
}
