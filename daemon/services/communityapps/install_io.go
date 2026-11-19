package communityapps

import "io"

type durableFileWriter interface {
	io.WriteCloser
	Sync() error
}

// writeAndSyncInstallFile closes the staging file even on a failed write or
// sync. Only a clean write, sync and close allows publication to proceed.
func writeAndSyncInstallFile(file durableFileWriter, data []byte) error {
	_, err := file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
