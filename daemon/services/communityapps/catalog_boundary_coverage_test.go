package communityapps_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestCatalogFailsClosedWhenHostStateCannotBeVerified(t *testing.T) {
	const osVersion = "etc/unraid-version"
	const manifest = "boot/config/plugins/community.applications.plg"
	const settings = "boot/config/plugins/community.applications/community.applications.cfg"
	const accepted = "boot/config/plugins/community.applications/accepted"

	for _, tc := range []struct {
		name, file, content string
		openError           error
		readError           error
		beyondStat          bool
		cancel              bool
		resolve             bool
		want                error
	}{
		{name: "missing OS version", file: osVersion, openError: fs.ErrNotExist, want: communityapps.ErrCatalogUnsupported},
		{name: "malformed OS version file", file: osVersion, content: "[", want: communityapps.ErrCatalogUnsupported},
		{name: "unrecognized OS version", file: osVersion, content: "version=unknown", want: communityapps.ErrCatalogUnsupported},
		{name: "overflowing OS version", file: osVersion, content: "version=" + strings.Repeat("9", 40) + ".2.0", want: communityapps.ErrCatalogUnsupported},
		{name: "unreadable CA settings", file: settings, openError: fs.ErrPermission, want: communityapps.ErrCatalogUnavailable},
		{name: "unreadable onboarding state", file: accepted, openError: fs.ErrPermission, resolve: true, want: communityapps.ErrCatalogUnavailable},
		{name: "catalog read fails after successful stat", file: cacheFile, readError: io.ErrUnexpectedEOF, want: communityapps.ErrCatalogUnavailable},
		{name: "cache grew beyond the stat size", file: cacheFile, beyondStat: true, want: communityapps.ErrCatalogUnsupported},
		{name: "cancelled while checking installed CA version", file: manifest, cancel: true, want: context.Canceled},
		{name: "cancelled while reading catalog", file: cacheFile, cancel: true, want: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			if tc.content != "" {
				communityappstest.WriteFile(t, root, tc.file, tc.content)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			host := catalogFaultFS{
				FS: os.DirFS(root), file: tc.file, openError: tc.openError, readError: tc.readError, beyondStat: tc.beyondStat,
			}
			if tc.cancel {
				host.onRead = cancel
			}
			catalog := communityapps.NewCatalog(host)
			var err error
			if tc.resolve {
				_, err = catalog.Resolve(ctx, communityappstest.HeimdallReference)
			} else {
				_, err = catalog.Search(ctx, "heimdall")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("catalog returned %v, want %v", err, tc.want)
			}
		})
	}
}

type catalogFaultFS struct {
	fs.FS
	file       string
	openError  error
	readError  error
	beyondStat bool
	onRead     func()
}

func (f catalogFaultFS) Open(name string) (fs.File, error) {
	if name != f.file {
		return f.FS.Open(name)
	}
	if f.openError != nil {
		return nil, f.openError
	}
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return catalogFaultFile{File: file, failure: f.readError, beyondStat: f.beyondStat, onRead: f.onRead}, nil
}

type catalogFaultFile struct {
	fs.File
	failure    error
	beyondStat bool
	onRead     func()
}

func (f catalogFaultFile) Read(dst []byte) (int, error) {
	if f.failure != nil {
		return 0, f.failure
	}
	if f.beyondStat {
		clear(dst)
		return len(dst), nil
	}
	if f.onRead != nil {
		f.onRead()
	}
	return f.File.Read(dst)
}

func TestSearchRejectsMalformedRowsAndAmbiguousBranchIDs(t *testing.T) {
	for _, data := range []string{
		`{}`, `a:1:{i:123456`, `a:1:{i:0;a:1:{i:0;d:123456`, `a:1:{i:0;a:1:{i:0;N;`,
	} {
		t.Run(data, func(t *testing.T) {
			root := communityappstest.CopyHost(t)
			communityappstest.WriteFile(t, root, cacheFile, data)
			if _, err := communityapps.NewCatalog(os.DirFS(root)).Search(t.Context(), ""); !errors.Is(err, communityapps.ErrCatalogUnsupported) {
				t.Fatalf("malformed catalog returned %v", err)
			}
		})
	}
	for _, field := range []string{"Repository", "Blacklist"} {
		t.Run("absent "+field, func(t *testing.T) {
			root := communityappstest.CopyHost(t)
			entry := testCatalogEntry()
			delete(entry, field)
			writeTestCatalog(t, root, entry)
			if _, err := communityapps.NewCatalog(os.DirFS(root)).Search(t.Context(), ""); !errors.Is(err, communityapps.ErrCatalogUnsupported) {
				t.Fatalf("incomplete host catalog returned %v", err)
			}
		})
	}
	t.Run("duplicate variant ID", func(t *testing.T) {
		root := communityappstest.CopyHost(t)
		parent := testCatalogEntry()
		parent["BranchID"] = []any{2}
		first, second := maps.Clone(parent), maps.Clone(parent)
		first["ID"], second["ID"] = 2, 2
		first["BranchName"], second["BranchName"] = "beta", "edge"
		writeTestCatalog(t, root, parent, first, second)
		result, err := communityapps.NewCatalog(os.DirFS(root)).Search(t.Context(), "")
		if err != nil || len(result.Listings) != 1 || result.Listings[0].Variant != "" {
			t.Fatalf("ambiguous branch IDs should be omitted: %+v, %v", result, err)
		}
	})
}

func testCatalogEntry() map[string]any {
	return map[string]any{
		"ID": 1, "Name": "sample", "Repository": "example/sample:latest",
		"TemplateURL": "https://example.invalid/sample.xml", "Author": "Example",
		"RepoName": "example repo", "Network": "bridge", "Displayable": true,
		"Compatible": true, "Blacklist": false, "Deprecated": false,
	}
}

func writeTestCatalog(t *testing.T, root string, entries ...map[string]any) {
	t.Helper()
	var encoded strings.Builder
	list := make([]any, 0, len(entries))
	for _, row := range entries {
		list = append(list, row)
	}
	writePHPFixture(t, &encoded, list)
	communityappstest.WriteFile(t, root, cacheFile, encoded.String())
}

func writePHPFixture(t *testing.T, dst *strings.Builder, value any) {
	t.Helper()
	switch v := value.(type) {
	case string:
		fmt.Fprintf(dst, `s:%d:"%s";`, len(v), v)
	case bool:
		if v {
			dst.WriteString("b:1;")
		} else {
			dst.WriteString("b:0;")
		}
	case int:
		fmt.Fprintf(dst, "i:%d;", v)
	case map[string]any:
		fmt.Fprintf(dst, "a:%d:{", len(v))
		for _, field := range slices.Sorted(maps.Keys(v)) {
			writePHPFixture(t, dst, field)
			writePHPFixture(t, dst, v[field])
		}
		dst.WriteByte('}')
	case []any:
		fmt.Fprintf(dst, "a:%d:{", len(v))
		for i, item := range v {
			writePHPFixture(t, dst, i)
			writePHPFixture(t, dst, item)
		}
		dst.WriteByte('}')
	default:
		t.Fatalf("unsupported PHP fixture value %T", value)
	}
}
