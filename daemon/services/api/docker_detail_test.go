package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

const dockerDetailTemplateDir = "boot/config/plugins/dockerMan/templates-user/"

func TestDockerDetailResolvesReplacementBeforeCollectorRefresh(t *testing.T) {
	server, _ := setupTestServer()
	root := t.TempDir()
	server.SetCommunityAppsService(communityapps.NewService(nil, communityapps.ServiceOptions{HostRoot: root}))
	communityappstest.WriteFile(t, root, dockerDetailTemplateDir+"my-heimdall.xml", `<Container version="2">
		<Name>heimdall</Name>
		<Repository>lscr.io/linuxserver/heimdall</Repository>
		<TemplateURL>https://raw.githubusercontent.com/linuxserver/templates/main/unraid/heimdall.xml</TemplateURL>
	</Container>`)
	current := `{
		"Id":"2222222222221111111111111111111111111111111111111111111111111111",
		"Name":"/heimdall",
		"State":{"Status":"running","Running":true,"StartedAt":"2026-11-18T00:00:00Z"},
		"Config":{"Image":"lscr.io/linuxserver/heimdall","Labels":{"net.unraid.docker.managed":"dockerman"}},
		"HostConfig":{"NetworkMode":"my-bridge","Memory":67108864,"RestartPolicy":{"Name":"unless-stopped"},"PortBindings":{"80/tcp":[{"HostIp":"127.0.0.1","HostPort":"18081"}]}},
		"Mounts":[{"Source":"/mnt/user/appdata/heimdall","Destination":"/config","Mode":"rw"}],
		"NetworkSettings":{"Networks":{"my-bridge":{"NetworkID":"bridge-id","IPAddress":"192.0.2.10"}},
			"Ports":{"80/tcp":[{"HostIp":"127.0.0.1","HostPort":"18081"}]}}
	}`
	communityappstest.UseDockerInspectEngine(t, map[string]string{
		"heimdall": current, "222222222222": current,
		"2222222222221111111111111111111111111111111111111111111111111111": current,
	})
	cache := []dto.ContainerInfo{{ID: "111111111111", Name: "heimdall", Image: "sha256:old", MemoryUsage: 999}}
	server.dockerCache.Store(&cache)

	for _, ref := range []string{"heimdall", "222222222222", "2222222222221111111111111111111111111111111111111111111111111111"} {
		t.Run(ref, func(t *testing.T) {
			response := dockerDetailRequest(server, ref)
			if response.Code != http.StatusOK {
				t.Fatalf("Docker detail status = %d: %s", response.Code, response.Body.String())
			}
			var result struct {
				dto.ContainerInfo
				CatalogLink string `json:"catalog_link"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.ID != "222222222222" || result.Name != "heimdall" || result.State != "running" ||
				result.Image != "lscr.io/linuxserver/heimdall" || result.MemoryUsage != 0 ||
				result.CatalogLink != "https://raw.githubusercontent.com/linuxserver/templates/main/unraid/heimdall.xml" {
				t.Fatalf("detail did not describe the live CA container: %+v", result)
			}
			if result.NetworkMode != "my-bridge" || result.IPAddress != "192.0.2.10" || result.RestartPolicy != "unless-stopped" ||
				result.ConfiguredMemoryLimit == nil || *result.ConfiguredMemoryLimit != 67108864 ||
				len(result.Ports) != 1 || result.Ports[0].PublicPort != 18081 ||
				len(result.PortMappings) != 1 || result.PortMappings[0] != "18081:80/tcp" ||
				len(result.VolumeMappings) != 1 || result.VolumeMappings[0].HostPath != "/mnt/user/appdata/heimdall" {
				t.Fatalf("replacement settings were unavailable before collection: %+v", result.ContainerInfo)
			}
		})
	}
	if response := dockerDetailRequest(server, "111111111111"); response.Code != http.StatusNotFound {
		t.Fatalf("stale ID was still returned: %d %s", response.Code, response.Body.String())
	}
}

func dockerDetailRequest(server *Server, ref string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/docker/"+ref, nil))
	return response
}

func TestDockerDetailUsesOnlySafeNativeTemplateProvenance(t *testing.T) {
	const publicLink = "https://example.com/templates/heimdall.xml"
	const managed = `{"Id":"2222222222220001","Name":"/heimdall","Config":{"Image":"lscr.io/linuxserver/heimdall:latest","Env":["SECRET=fixture-secret"],"Labels":{"net.unraid.docker.managed":"dockerman"}},"State":{"Status":"exited"}}` // pragma: allowlist secret (synthetic fixture value)
	const unmanaged = `{"Id":"3333333333330002","Name":"/heimdall-b","Config":{"Image":"lscr.io/linuxserver/heimdall"},"State":{"Status":"running"}}`
	communityappstest.UseDockerInspectEngine(t, map[string]string{"heimdall": managed, "heimdall-b": unmanaged})

	for _, tc := range []struct {
		name, file, xml, ref, want string
		duplicate                  bool
		symlink                    bool
	}{
		{name: "arbitrary native filename", file: "alternate.xml", xml: nativeOriginFixture("heimdall", publicLink), want: publicLink},
		{name: "nested native filename", file: "saved/alternate.xml", xml: nativeOriginFixture("heimdall", publicLink), want: publicLink},
		{name: "hidden directory", file: ".backup/alternate.xml", xml: nativeOriginFixture("heimdall", publicLink)},
		{name: "ambiguous XML name", xml: nativeOriginFixture("heimdall", publicLink), duplicate: true},
		{name: "different name same image", xml: nativeOriginFixture("heimdall-b", publicLink)},
		{name: "unmanaged same image", ref: "heimdall-b", xml: nativeOriginFixture("heimdall-b", publicLink)},
		{name: "no template"},
		{name: "no link", xml: "<Container><Name>heimdall</Name></Container>"},
		{name: "empty link", xml: nativeOriginFixture("heimdall", "")},
		{name: "invalid scheme", xml: nativeOriginFixture("heimdall", "file:///fixture-secret")},
		{name: "URL credentials", xml: nativeOriginFixture("heimdall", "https://user:fixture-secret@example.com/heimdall.xml")},
		{name: "URL query token", xml: nativeOriginFixture("heimdall", publicLink+"?token=fixture-secret")},
		{name: "URL fragment token", xml: nativeOriginFixture("heimdall", publicLink+"#fixture-secret")},
		{name: "WebUI placeholder is not a catalog host", xml: nativeOriginFixture("heimdall", "http://[IP]:[PORT:80]")},
		{name: "unclosed XML", xml: nativeOriginFixture("heimdall", publicLink) + "<"},
		{name: "multiple roots", xml: nativeOriginFixture("heimdall", publicLink) + "<Container/>"},
		{name: "trailing text", xml: nativeOriginFixture("heimdall", publicLink) + "fixture-secret"},
		{name: "external entity declaration", xml: `<!DOCTYPE Container SYSTEM "file:///fixture-secret">` + nativeOriginFixture("heimdall", publicLink)},
		{name: "duplicate link elements", xml: "<Container><Name>heimdall</Name><TemplateURL>" + publicLink + "</TemplateURL><TemplateURL>fixture-secret</TemplateURL></Container>"},
		{name: "oversized file", xml: nativeOriginFixture("heimdall", publicLink) + strings.Repeat(" ", 1<<20)},
		{name: "symlinked template", xml: nativeOriginFixture("heimdall", publicLink), symlink: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := setupTestServer()
			root := t.TempDir()
			server.SetCommunityAppsService(communityapps.NewService(nil, communityapps.ServiceOptions{HostRoot: root}))
			filename := tc.file
			if filename == "" {
				filename = "my-heimdall.xml"
			}
			if tc.symlink {
				writeSymlinkedTemplate(t, root, filename, tc.xml)
			} else if tc.xml != "" {
				communityappstest.WriteFile(t, root, dockerDetailTemplateDir+filename, tc.xml)
			}
			if tc.duplicate {
				communityappstest.WriteFile(t, root, dockerDetailTemplateDir+"second.xml", tc.xml)
			}
			ref := tc.ref
			if ref == "" {
				ref = "heimdall"
			}
			response := dockerDetailRequest(server, ref)
			var detail dto.ContainerDetail
			if err := json.Unmarshal(response.Body.Bytes(), &detail); response.Code != http.StatusOK || err != nil {
				t.Fatalf("invalid lookup response (%d): %s", response.Code, response.Body.String())
			}
			if detail.CatalogLink != tc.want || detail.Name != ref || strings.Contains(response.Body.String(), "fixture-secret") {
				t.Fatalf("unexpected catalog provenance: %s", response.Body.String())
			}
			if tc.want == "" && strings.Contains(response.Body.String(), "catalog_link") {
				t.Fatalf("unknown origin did not omit catalog_link: %s", response.Body.String())
			}
		})
	}
}

func TestDockerDetailDoesNotReturnStaleIdentityOnEngineFailure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T)
	}{
		{name: "invalid Docker host", setup: func(t *testing.T) {
			t.Setenv("DOCKER_HOST", "invalid")
		}},
		{name: "unavailable Docker", setup: func(t *testing.T) {
			t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(t.TempDir(), "missing.sock"))
		}},
		{name: "malformed Docker response", setup: func(t *testing.T) {
			communityappstest.UseDockerInspectEngine(t, map[string]string{"heimdall": `{"Id":`})
		}},
		{name: "incomplete response", setup: func(t *testing.T) {
			communityappstest.UseDockerInspectEngine(t, map[string]string{"heimdall": `{"Id":"2222222222220001","Name":"/heimdall"}`})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			server, _ := setupTestServer()
			cache := []dto.ContainerInfo{{ID: "111111111111", Name: "heimdall"}}
			server.dockerCache.Store(&cache)
			response := dockerDetailRequest(server, "heimdall")
			if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "111111111111") {
				t.Fatalf("expected live identity failure, got %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDockerDetailRejectsInvalidReferenceBeforeOpeningEngine(t *testing.T) {
	t.Setenv("DOCKER_HOST", "invalid")
	server, _ := setupTestServer()
	t.Cleanup(server.Stop)
	response := dockerDetailRequest(server, "invalid!reference")
	if response.Code != http.StatusNotFound {
		t.Fatalf("invalid container reference returned %d: %s", response.Code, response.Body)
	}
}

func nativeOriginFixture(name, link string) string {
	return "<Container><Name>" + name + "</Name><Repository>lscr.io/linuxserver/heimdall</Repository><TemplateURL>" + link + "</TemplateURL></Container>"
}

func writeSymlinkedTemplate(t *testing.T, root, name, content string) {
	t.Helper()
	communityappstest.WriteFile(t, root, "outside.xml", content)
	dir := filepath.Join(root, dockerDetailTemplateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.xml"), filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}
