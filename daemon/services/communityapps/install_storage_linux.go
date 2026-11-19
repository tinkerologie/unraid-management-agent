package communityapps

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func mkdirInstallDirectory(parent *os.File, name string, native bool) error {
	fd := int(parent.Fd())
	if !native {
		return unix.Mkdirat(fd, name, 0o777)
	}

	// Native Docker Manager gives new appdata to nobody:users. Create
	// directly with that filesystem identity so there is no path-based
	// ownership change after mkdir in a writable share.
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		err, restored := mkdirWithNativeOwner(fd, name)
		if restored {
			runtime.UnlockOSThread()
		}
		// Go discards a thread when its locked goroutine exits.
		done <- err
	}()
	return <-done
}

func mkdirWithNativeOwner(fd int, name string) (err error, restored bool) {
	const nobody, users = 99, 100
	uid, err := unix.SetfsuidRetUid(-1)
	if err != nil {
		return err, false
	}
	gid, err := unix.SetfsgidRetGid(-1)
	if err != nil {
		return err, false
	}
	defer func() {
		_, uidErr := unix.SetfsuidRetUid(uid)
		_, gidErr := unix.SetfsgidRetGid(gid)
		actualUID, uidCheck := unix.SetfsuidRetUid(-1)
		actualGID, gidCheck := unix.SetfsgidRetGid(-1)
		restored = uidErr == nil && gidErr == nil && uidCheck == nil && gidCheck == nil &&
			actualUID == uid && actualGID == gid
		if !restored {
			err = ErrStorageState
		}
	}()

	// Linux may silently reject these changes. Verify the effective IDs
	// before performing any filesystem operation.
	if _, err = unix.SetfsgidRetGid(users); err != nil {
		return err, false
	}
	if _, err = unix.SetfsuidRetUid(nobody); err != nil {
		return err, false
	}
	actualUID, uidCheck := unix.SetfsuidRetUid(-1)
	actualGID, gidCheck := unix.SetfsgidRetGid(-1)
	if uidCheck != nil || gidCheck != nil || actualUID != nobody || actualGID != users {
		return ErrStorageState, false
	}
	return unix.Mkdirat(fd, name, 0o777), false
}
