package plugininventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUnraidVersionBoundsMatchPHP(t *testing.T) {
	// Expected signs recorded from PHP 8 version_compare.
	for _, tc := range []struct {
		left, right string
		sign        int
	}{
		{"7.10", "7.9", 1}, {"7.4.0-rc.1", "7.4.0", -1}, {"7.4", "7.4.0", -1},
		{"7.4.0+pl1", "7.4.0", 1}, {"7.4.0_beta2", "7.4.0RC1", -1},
		{"7.4.0", "7.4.0.0", -1}, {"7.4.0dev", "7.4.0beta1", -1},
		{"7.4.0", "7.4.0", 0},
	} {
		if got := compareOSVersions(tc.left, tc.right); got != tc.sign {
			t.Errorf("compare %s and %s = %d, want %d", tc.left, tc.right, got, tc.sign)
		}
	}
}

func TestManifestRejectsUnsafeEntityExpansion(t *testing.T) {
	for _, tc := range []struct {
		name, manifest string
	}{
		{"external reference", `<!DOCTYPE PLUGIN [<!ENTITY ext SYSTEM "file:///fixture.xml">]><PLUGIN author="&ext;"/>`},
		{"entity cycle", `<!DOCTYPE PLUGIN [<!ENTITY a "&b;"><!ENTITY b "&a;">]><PLUGIN author="&a;"/>`},
		{"document expansion", `<!DOCTYPE PLUGIN [<!ENTITY a "` + strings.Repeat("x", 4096) + `">]><PLUGIN author="` + strings.Repeat("&a;", 260) + `"/>`},
		{"malformed", `<PLUGIN name="bad"><FILE></PLUGIN>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sample.plg")
			if err := os.WriteFile(path, []byte(tc.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := parseManifest(path); err == nil {
				t.Fatal("invalid manifest was accepted")
			}
		})
	}
}

func TestResolverUsesNativePluginInventoryAndUpdateRules(t *testing.T) {
	root := t.TempDir()
	runtimeDir, bootDir, updates := filepath.Join(root, "runtime"), filepath.Join(root, "boot"), filepath.Join(root, "updates")
	for _, dir := range []string{runtimeDir, bootDir, updates} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(file, contents string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := func(file, target string) {
		t.Helper()
		if err := os.Symlink(target, file); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"available", "incompatible", "invalid", "stale", "fresh", "malformed", "absent", "unRAIDServer", "boot-only"} {
		manifest := filepath.Join(bootDir, name+".plg")
		write(manifest, fmt.Sprintf(`<!DOCTYPE PLUGIN [<!ENTITY ver '2026.10.01'><!ENTITY key '%s'><!ENTITY base 'https://example.test'>]>
<PLUGIN name="&key;" version="&ver;" pluginURL="&base;/candidate-%s.plg"><CHANGES><![CDATA[a > b]]></CHANGES></PLUGIN>`, name, name))
		if name != "boot-only" {
			link(filepath.Join(runtimeDir, name+".plg"), manifest)
		}
		switch name {
		case "available", "incompatible", "invalid":
			minimum := "7.4.0"
			if name == "incompatible" {
				minimum = "7.5.0"
			}
			write(filepath.Join(updates, "candidate-"+name+".plg"), fmt.Sprintf(`<PLUGIN version="2026.11.01" min="%s"/>`, minimum))
		case "malformed":
			write(filepath.Join(updates, "candidate-malformed.plg"), `<PLUGIN broken`)
		case "stale", "fresh":
			write(filepath.Join(updates, "candidate-"+name+".plg"), `<PLUGIN version="2026.09.01"/>`)
		}
	}
	link(filepath.Join(runtimeDir, "dangling.plg"), filepath.Join(bootDir, "missing.plg"))
	write(filepath.Join(runtimeDir, "regular.plg"), `<PLUGIN version="2026.10.01"/>`)
	write(filepath.Join(updates, "invalid.plg.invalid"), `{"version":"2026.11.02","summary":"Integrity check failed"}`)
	versionPath := filepath.Join(root, "unraid-version")
	write(versionPath, "version=\"7.4.0\"\n")

	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	resolver := Resolver{
		RuntimeDir: runtimeDir, CandidateDir: updates, OSVersionPath: versionPath,
		Now: func() time.Time { return now },
		changeTime: func(info os.FileInfo) time.Time {
			if info.Name() == "candidate-stale.plg" {
				return now.Add(-25 * time.Hour)
			}
			return now.Add(-time.Hour)
		},
	}
	targets, err := resolver.CheckTargets(t.Context())
	if err != nil || !slices.Equal(targets, []string{"absent.plg", "available.plg", "fresh.plg", "incompatible.plg", "invalid.plg", "malformed.plg", "stale.plg"}) {
		t.Fatalf("unexpected registered refresh targets %v: %v", targets, err)
	}
	got, err := resolver.Resolve(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"available": "update_available", "incompatible": "update_incompatible", "invalid": "validation_failed",
		"stale": "need_check", "fresh": "up_to_date", "malformed": "need_check", "absent": "not_available",
	}
	if got.TotalCount != len(want) || got.UpdatesAvailable != 1 || !got.Timestamp.Equal(now) {
		t.Fatalf("unexpected plugin totals: %+v", got)
	}
	for _, item := range got.Plugins {
		if item.UpdateStatus != want[item.Name] || item.UpdateAvailable != (item.Name == "available") {
			t.Fatalf("unexpected status: %+v", item)
		}
		if item.Name == "invalid" && (item.LatestVersion != "2026.11.02" || item.ValidationMessage != "Integrity check failed") {
			t.Fatalf("validation details missing: %+v", item)
		}
		delete(want, item.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing plugins: %v", want)
	}
}
