package communityapps_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestSearchWithNoHostCatalog(t *testing.T) {
	catalog := communityapps.NewCatalog(fstest.MapFS{})
	_, err := catalog.Search(t.Context(), "redis")
	if !errors.Is(err, communityapps.ErrCatalogUnavailable) {
		t.Fatalf("Search returned %v, want catalog unavailable", err)
	}
}

func TestSearchSelectsExactVisibleVariant(t *testing.T) {
	catalog := fixtureCatalog(t)
	result, err := catalog.Search(t.Context(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if result.CAVersion != "2026.07.21" || len(result.Listings) != 2 {
		t.Fatalf("expected two visible Docker listings from 2026.07.21, got %+v", result)
	}
	base, edge := result.Listings[0], result.Listings[1]
	if base.Name != "Fixture Café" || edge.Variant != "edge" || base.Image != edge.Image || base.Reference == edge.Reference {
		t.Fatalf("fixture listings did not distinguish variants with identical images: %+v", result.Listings)
	}
	resolved, err := catalog.Resolve(t.Context(), edge.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Listing.Variant != "edge" || resolved.Template.Name != "Fixture Edge" {
		t.Fatal("reference did not select the exact branch template")
	}
}

func fixtureCatalog(t *testing.T) *communityapps.Catalog {
	t.Helper()
	return communityapps.NewCatalog(os.DirFS(communityappstest.CopyHost(t)))
}

const (
	cacheFile         = communityappstest.CachePath
	pluginFile        = "boot/config/plugins/community.applications.plg"
	settingsDirectory = "boot/config/plugins/community.applications"
)

func TestSearchCatalogStates(t *testing.T) {
	t.Run("empty search is a successful collection", func(t *testing.T) {
		catalog := fixtureCatalog(t)
		result, err := catalog.Search(t.Context(), "no match")
		if err != nil || result.Listings == nil || len(result.Listings) != 0 || result.Total != 0 {
			t.Fatalf("search returned %+v, %v; want an empty collection", result, err)
		}
	})
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, root string)
		want   error
	}{
		{"cold cache", func(t *testing.T, root string) {
			t.Helper()
			if err := os.Remove(filepath.Join(root, cacheFile)); err != nil {
				t.Fatal(err)
			}
		}, communityapps.ErrCatalogUnavailable},
		{"unsupported build", func(t *testing.T, root string) {
			t.Helper()
			writeFixtureFile(t, root, pluginFile, []byte(`<!ENTITY version "2026.11.19">`))
		}, communityapps.ErrCatalogUnsupported},
		{"Unraid below baseline", func(t *testing.T, root string) {
			t.Helper()
			writeFixtureFile(t, root, "etc/unraid-version", []byte(`version="7.1.9"`))
		}, communityapps.ErrCatalogUnsupported},
		{"unsupported cache", func(t *testing.T, root string) {
			t.Helper()
			writeFixtureFile(t, root, cacheFile, []byte(`a:1:{i:0;O:4:"Demo":0:{}}`))
		}, communityapps.ErrCatalogUnsupported},
		{"invalid settings", func(t *testing.T, root string) {
			t.Helper()
			writeFixtureFile(t, root, settingsDirectory+"/community.applications.cfg", []byte("["))
		}, communityapps.ErrCatalogUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHost(t)
			tc.change(t, root)
			result, err := communityapps.NewCatalog(os.DirFS(root)).Search(t.Context(), "fixture")
			if !errors.Is(err, tc.want) {
				t.Fatalf("search returned %+v, %v; want %v", result, err, tc.want)
			}
		})
	}
}

func TestInspectRedactsPublishedSettingsAndExplainsEligibility(t *testing.T) {
	catalog := fixtureCatalog(t)
	search, err := catalog.Search(t.Context(), "edge")
	if err != nil || len(search.Listings) != 1 {
		t.Fatalf("search = %+v, %v", search, err)
	}
	inspection, err := catalog.Inspect(t.Context(), search.Listings[0].Reference)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Network != "bridge" || inspection.OnboardingAccepted || len(inspection.Fields) != 3 ||
		!slices.Contains(inspection.EligibilityIssues, "raw_docker_arguments") ||
		!slices.Contains(inspection.EligibilityIssues, "accept_warning_in_unraid_apps") {
		t.Fatalf("inspection did not report the declared fields and known blockers: %+v", inspection)
	}
	port, volume, env := inspection.Fields[0], inspection.Fields[1], inspection.Fields[2]
	if port.Target != "8080" || port.Value != "18081" || port.Default != "18080" || port.Mode != "tcp" || !port.Required ||
		volume.Target != "/config" || volume.Default != "/mnt/user/appdata/fixture" ||
		env.Target != "API_TOKEN" || !env.HasValue || !env.HasDefault || env.Value != "" || env.Default != "" {
		t.Fatalf("inspection returned unexpected fields: %+v", inspection.Fields)
	}
	resolved, err := catalog.Resolve(t.Context(), inspection.Listing.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Template.Config[2].Value != "fixture-value-sentinel" || resolved.Template.CatalogLink != "https://example.invalid/templates/fixture.xml" {
		t.Fatal("typed template lost the native catalog link or environment value")
	}
	// Public results are safe even if internal template types are marshaled accidentally.
	for _, payload := range []any{inspection, resolved} {
		data, err := json.Marshal(payload)
		if err != nil || bytes.Contains(data, []byte("sentinel")) {
			t.Fatal("serialized results disclose a template secret")
		}
	}
}

func TestResolveUsesCurrentCatalogAndPolicy(t *testing.T) {
	root := communityappstest.CopyHost(t)
	catalog := communityapps.NewCatalog(os.DirFS(root))
	writeFixtureFile(t, root, settingsDirectory+"/community.applications.cfg", []byte("hideDeprecated=\"false\"\nhideIncompatible=\"false\"\n"))
	writeFixtureFile(t, root, settingsDirectory+"/accepted", nil)

	search, err := catalog.Search(t.Context(), "fixture")
	if err != nil || len(search.Listings) != 4 {
		t.Fatalf("local CA settings did not show deprecated and incompatible listings: %+v, %v", search, err)
	}
	edge, incompatible := search.Listings[1], search.Listings[3]
	inspection, err := catalog.Inspect(t.Context(), incompatible.Reference)
	if err != nil || !inspection.OnboardingAccepted || !slices.Contains(inspection.EligibilityIssues, "incompatible_unraid_version") {
		t.Fatalf("expected discoverable incompatible listing with an eligibility reason: %+v, %v", inspection, err)
	}

	writeFixtureFile(t, root, settingsDirectory+"/community.applications.cfg", []byte("hideDeprecated=\"true\"\nhideIncompatible=\"true\"\n"))
	if _, err := catalog.Resolve(t.Context(), incompatible.Reference); !errors.Is(err, communityapps.ErrListingUnavailable) {
		t.Fatalf("Resolve returned %v for a now-hidden listing", err)
	}

	original, err := os.ReadFile(filepath.Join(root, cacheFile))
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Replace(original, []byte(`s:2:"ID";i:11;`), []byte(`s:2:"ID";i:99;`), 1)
	data = bytes.Replace(data, []byte(`a:2:{i:0;i:11;i:1;i:12;}`), []byte(`a:2:{i:0;i:99;i:1;i:12;}`), 1)
	if err := os.WriteFile(filepath.Join(root, cacheFile+".next"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, cacheFile+".next"), filepath.Join(root, cacheFile)); err != nil {
		t.Fatal(err)
	}
	resolved, err := catalog.Resolve(t.Context(), edge.Reference)
	if err != nil || resolved.Listing.Variant != "edge" {
		t.Fatalf("changed transient CA ID lost the original variant: %+v, %v", resolved.Listing, err)
	}

	hidden := bytes.Replace(data, []byte(`s:9:"Blacklist";b:0;`), []byte(`s:9:"Blacklist";b:1;`), 1)
	writeFixtureFile(t, root, cacheFile, hidden)
	if _, err := catalog.Resolve(t.Context(), edge.Reference); !errors.Is(err, communityapps.ErrListingUnavailable) {
		t.Fatalf("Resolve returned %v for a newly blacklisted listing", err)
	}
}

func TestInspectVerifiedHostCacheSample(t *testing.T) {
	root := communityappstest.CopyHost(t)
	sample, err := os.ReadFile("testdata/heimdall-2026.07.21.serialized.txt")
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, cacheFile, sample)
	catalog := communityapps.NewCatalog(os.DirFS(root))
	search, err := catalog.Search(t.Context(), "heimdall")
	if err != nil || len(search.Listings) != 2 {
		t.Fatalf("search did not read the default and development variants: %+v, %v", search, err)
	}
	variant := search.Listings[1]
	inspection, err := catalog.Inspect(t.Context(), variant.Reference)
	if err != nil || variant.Variant != "development" || len(inspection.Fields) != 7 {
		t.Fatalf("inspection did not read the development variant: %+v, %v", inspection, err)
	}
}

func writeFixtureFile(t *testing.T, root, name string, data []byte) {
	t.Helper()
	filename := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogRejectsMalformedSerialization(t *testing.T) {
	for _, tc := range []struct {
		name, data string
	}{
		{"negative count", "a:-1:{}"},
		{"incomplete count", "a:1{}"},
		{"array as a key", "a:1:{a:0:{}s:1:\"x\";}"},
		{"null as a key", "a:1:{N;s:1:\"x\";}"},
		{"empty row", "a:1:{i:1;a:0:{}}"},
		{"scalar row", "a:1:{i:1;s:2:\"ok\";}"},
		{"invalid length", "a:1:{i:0;a:1:{s:x:\"x\";N;}}"},
		{"missing string terminator", "a:1:{i:0;a:1:{s:1:\"x :5;N;}}"},
		{"excess nesting", "a:1:{i:0;" + strings.Repeat("a:1:{i:0;", 34) + "N;" + strings.Repeat("}", 35)},
		{"trailing data", "a:0:{}N;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHost(t)
			writeFixtureFile(t, root, cacheFile, []byte(tc.data))
			_, err := communityapps.NewCatalog(os.DirFS(root)).Search(t.Context(), "")
			if !errors.Is(err, communityapps.ErrCatalogUnsupported) {
				t.Fatalf("search returned %v, want unsupported", err)
			}
		})
	}
}

func TestCatalogValidatesRequest(t *testing.T) {
	catalog := fixtureCatalog(t)
	if _, err := catalog.Search(t.Context(), strings.Repeat("q", 257)); !errors.Is(err, communityapps.ErrInvalidSearch) {
		t.Fatalf("long query returned %v", err)
	}
	for _, ref := range []string{"../etc/passwd", strings.Repeat("z", 64)} {
		if _, err := catalog.Inspect(t.Context(), ref); !errors.Is(err, communityapps.ErrInvalidReference) {
			t.Fatalf("invalid reference returned %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := catalog.Search(ctx, "fixture"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled search returned %v", err)
	}
}

func TestInspectEffectiveUnsupportedSettings(t *testing.T) {
	root := communityappstest.CopyHost(t)
	data, err := os.ReadFile("testdata/runtime-2026.07.21.serialized.txt")
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, cacheFile, data)
	catalog := communityapps.NewCatalog(os.DirFS(root))
	for _, tc := range []struct {
		query  string
		fields int
		issues []string
	}{
		{"modern", 1, []string{
			"privileged", "device_passthrough", "unsupported_config", "invalid_environment_name",
			"docker_socket_mount", "unsupported_network_settings", "advanced_runtime_settings",
			"uninstall_only", "publisher_requirements", "private_template",
		}},
		{"legacy", 0, []string{"unsupported_network", "legacy_runtime_settings"}},
		{"single", 1, nil},
		{"malformed", 0, []string{"unsupported_config"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			search, err := catalog.Search(t.Context(), tc.query)
			if err != nil || len(search.Listings) != 1 {
				t.Fatalf("ineligible app must remain searchable: %+v, %v", search, err)
			}
			inspection, err := catalog.Inspect(t.Context(), search.Listings[0].Reference)
			if err != nil || len(inspection.Fields) != tc.fields {
				t.Fatalf("inspection returned unexpected fields: %+v, %v", inspection, err)
			}
			for _, reason := range tc.issues {
				if !slices.Contains(inspection.EligibilityIssues, reason) {
					t.Fatalf("expected eligibility issue %s, got %v", reason, inspection.EligibilityIssues)
				}
			}
			data, err := json.Marshal(inspection)
			if err != nil || bytes.Contains(data, []byte("sentinel")) {
				t.Fatal("inspection discloses an invalid environment declaration")
			}
		})
	}
}
