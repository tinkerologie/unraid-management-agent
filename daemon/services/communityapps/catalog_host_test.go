package communityapps

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestSearchRejectsOversizeHostCache(t *testing.T) {
	root := communityappstest.CopyHost(t)
	if err := os.Truncate(filepath.Join(root, catalogPath), maxCatalogBytes+1); err != nil {
		t.Fatal(err)
	}
	catalog := NewCatalog(hostFiles{root: root, owner: os.Getuid()})
	if _, err := catalog.Search(t.Context(), ""); !errors.Is(err, ErrCatalogUnsupported) {
		t.Fatalf("oversize cache returned %v, want unsupported", err)
	}
}

func TestReadBoundedRejectsLimitOverrun(t *testing.T) {
	files := fstest.MapFS{"cache": {Data: []byte("12345")}}
	file, err := files.Open("cache")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if data, err := readBounded(file, 4); data != nil || !errors.Is(err, errCacheEncoding) {
		t.Fatalf("over-limit read returned %q, %v, want encoding error", data, err)
	}
}

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
