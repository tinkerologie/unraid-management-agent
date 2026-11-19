package communityapps

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestNativeCatalogReaderLimitsHostPaths(t *testing.T) {
	root := communityappstest.CopyHost(t)
	communityappstest.WriteFile(t, root, "outside-catalog", "unrelated data")
	reader := hostFiles{root: root, owner: os.Getuid()}
	file, err := reader.Open("outside-catalog")
	if err == nil {
		_ = file.Close()
		t.Fatal("native catalog reader opened an unrelated host file")
	}
	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("unrelated host file error = %v", err)
	}
}
