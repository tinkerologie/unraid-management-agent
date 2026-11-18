package communityapps

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestSearchRejectsMutableAndLinkedHostCaches(t *testing.T) {
	for _, mode := range []string{"world-writable", "symbolic-link"} {
		t.Run(mode, func(t *testing.T) {
			root := communityappstest.CopyHost(t)
			catalog := NewCatalog(hostFiles{root: root, owner: os.Getuid()})
			if _, err := catalog.Search(t.Context(), "fixture"); err != nil {
				t.Fatalf("trusted fixtures should be readable: %v", err)
			}
			cache := filepath.Join(root, catalogPath)
			switch mode {
			case "world-writable":
				if err := os.Chmod(cache, 0o666); err != nil {
					t.Fatal(err)
				}
			case "symbolic-link":
				if err := os.Rename(cache, cache+".linked"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(cache+".linked", cache); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := catalog.Search(t.Context(), "fixture"); !errors.Is(err, ErrCatalogUnavailable) {
				t.Fatalf("unsafe cache returned %v, want unavailable", err)
			}
		})
	}
}
