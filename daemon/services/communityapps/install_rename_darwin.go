package communityapps

import "golang.org/x/sys/unix"

func renameExclusive(source, target string) error {
	return unix.RenamexNp(source, target, unix.RENAME_EXCL)
}
