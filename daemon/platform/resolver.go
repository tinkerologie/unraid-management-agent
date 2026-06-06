package platform

import (
	"os"
	"os/exec"
	"path/filepath"
)

// PathExists reports whether a filesystem path exists.
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// BinaryExists reports whether a binary is available — by absolute path if given,
// otherwise via PATH lookup of its base name.
func BinaryExists(pathOrName string) bool {
	if filepath.IsAbs(pathOrName) {
		if PathExists(pathOrName) {
			return true
		}
	}
	_, err := exec.LookPath(filepath.Base(pathOrName))
	return err == nil
}
