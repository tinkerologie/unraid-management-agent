package controllers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
)

func TestCALifecyclePreservesSettingsAndIsolatesSameListing(t *testing.T) {
	for _, tc := range []struct {
		name, failure string
		nativeEdit    bool
	}{
		{name: "agent install"},
		{name: "after native edit", nativeEdit: true},
		{name: "failed replacement", nativeEdit: true, failure: "start"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			appdata := filepath.Join(root, "heimdall-a")
			otherAppdata := filepath.Join(root, "heimdall-b")
			for _, dir := range []string{appdata, otherAppdata} {
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte(dir), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			template := filepath.Join(root, "my-fixture.xml")
			const nativeXML = "<Container><Name>fixture</Name><TemplateURL>https://example.com/heimdall.xml</TemplateURL></Container>"
			if err := os.WriteFile(template, []byte(nativeXML), 0o600); err != nil {
				t.Fatal(err)
			}
			engine := &updateTestEngine{
				originalName: "fixture", originalRunning: true, restartPolicy: "unless-stopped", failure: tc.failure,
				caAppdata: appdata, caNativeEdit: tc.nativeEdit,
			}
			server := httptest.NewServer(engine)
			t.Cleanup(server.Close)
			t.Setenv("DOCKER_HOST", server.URL)
			controller := NewDockerController()
			t.Cleanup(func() { _ = controller.Close() })

			if err := controller.Stop(updateOriginalID); err != nil {
				t.Fatal(err)
			}
			if err := controller.Start(updateOriginalID); err != nil {
				t.Fatal(err)
			}
			if err := controller.Restart(updateOriginalID); err != nil {
				t.Fatal(err)
			}
			result, err := controller.UpdateContainer("fixture", true)
			if tc.failure == "" {
				if err != nil || result == nil || !result.Recreated || result.ContainerID != "bbbbbbbbbbbb" {
					t.Fatalf("running CA replacement failed: %+v %v", result, err)
				}
				if err := controller.Remove(updateReplacementID, false); err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "original restored") || result != nil {
				t.Fatalf("failed CA replacement did not recover the original: %+v %v", result, err)
			}

			engine.mu.Lock()
			defer engine.mu.Unlock()
			if engine.caPull != "lscr.io/linuxserver/heimdall:latest" || engine.caReplacementName != "fixture" {
				t.Fatalf("update selected the wrong image or instance: %q %q", engine.caPull, engine.caReplacementName)
			}
			assertCAReplacementSettings(t, engine.created, appdata, tc.nativeEdit)
			wantActions := []string{
				"stop original", "start original", "restart original", "stop original", "retain original", "create replacement", "start replacement",
			}
			if tc.failure == "" {
				wantActions = append(wantActions, "remove original", "force-remove replacement")
				if !engine.originalRemoved || engine.replacement {
					t.Fatalf("removal left an agent-managed container: %+v", result)
				}
			} else {
				wantActions = append(wantActions, "force-remove replacement", "restore original", "start original")
				if engine.originalRemoved || engine.replacement || !engine.originalRunning ||
					engine.originalName != "fixture" || engine.restartPolicy != "unless-stopped" {
					t.Fatal("recovery did not restore the CA container's running identity and restart policy")
				}
			}
			if !slices.Equal(engine.caActions, wantActions) {
				t.Fatalf("unexpected lifecycle actions or destructive remove flags: %v", engine.caActions)
			}

			for filename, want := range map[string]string{
				template:                                nativeXML,
				filepath.Join(appdata, "sentinel"):      appdata,
				filepath.Join(otherAppdata, "sentinel"): otherAppdata,
			} {
				if data, err := os.ReadFile(filename); err != nil || string(data) != want {
					t.Fatalf("lifecycle changed retained state at %s: %v", filename, err)
				}
			}
		})
	}
}

func assertCAReplacementSettings(t *testing.T, request map[string]any, appdata string, nativeEdit bool) {
	t.Helper()
	var decoded struct {
		container.Config
		HostConfig       container.HostConfig
		NetworkingConfig network.NetworkingConfig
	}
	serialized, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(serialized, &decoded); err != nil {
		t.Fatal(err)
	}
	host, cfg := decoded.HostConfig, decoded.Config
	if cfg.Image != "lscr.io/linuxserver/heimdall:latest" || !slices.Equal(cfg.Env, []string{"ALLOW_INTERNAL_REQUESTS=false"}) ||
		cfg.Labels["net.unraid.docker.managed"] != "dockerman" || cfg.Labels["net.unraid.docker.webui"] != "http://[IP]:[PORT:80]" ||
		host.NetworkMode != "ca-bridge" || host.RestartPolicy.Name != container.RestartPolicyUnlessStopped {
		t.Fatal("recreation lost the configured image, native labels, environment, network, or restart policy")
	}
	ports := host.PortBindings[network.MustParsePort("80/tcp")]
	if len(ports) != 1 || ports[0].HostIP.String() != "127.0.0.1" || ports[0].HostPort != "18081" ||
		decoded.NetworkingConfig.EndpointsConfig["ca-bridge"] == nil {
		t.Fatalf("recreation changed the host publication or bridge: %v", ports)
	}
	if nativeEdit && !slices.Equal(host.Binds, []string{appdata + ":/config:rw"}) {
		t.Fatalf("recreation changed the native appdata bind: %v", host.Binds)
	}
	if !nativeEdit && !slices.ContainsFunc(host.Mounts, func(m mount.Mount) bool {
		return m.Type == mount.TypeBind && m.Source == appdata && m.Target == "/config" && !m.ReadOnly
	}) {
		t.Fatal("recreation lost the configured appdata mount")
	}
	if !slices.ContainsFunc(host.Mounts, func(m mount.Mount) bool {
		return m.Type == mount.TypeVolume && m.Source == "heimdall-existing-volume" && m.Target == "/data"
	}) {
		t.Fatal("recreation lost the existing volume")
	}
}

func (e *updateTestEngine) caInspectResponse(name string) string {
	hostMounts := fmt.Sprintf(`"Mounts":[{"Type":"bind","Source":%q,"Target":"/config"}],`, e.caAppdata)
	if e.caNativeEdit {
		hostMounts = fmt.Sprintf(`"Binds":[%q],`, e.caAppdata+":/config:rw")
	}
	return fmt.Sprintf(`{
		"Id":%q,"Name":%q,"Image":"sha256:old",
		"Config":{"Image":"lscr.io/linuxserver/heimdall:latest","Env":["ALLOW_INTERNAL_REQUESTS=false"],"Labels":{"net.unraid.docker.managed":"dockerman","net.unraid.docker.webui":"http://[IP]:[PORT:80]"}},
		"State":{"Running":%t},
		"HostConfig":{%s"NetworkMode":"ca-bridge","RestartPolicy":{"Name":%q},"PortBindings":{"80/tcp":[{"HostIp":"127.0.0.1","HostPort":"18081"}]}},
		"Mounts":[{"Type":"volume","Name":"heimdall-existing-volume","Destination":"/data","RW":true}],
		"NetworkSettings":{"Networks":{"ca-bridge":{"IPAddress":"198.51.100.2","Aliases":["fixture","aaaaaaaaaaaa"]}}}
	}`, updateOriginalID, "/"+name, e.originalRunning, hostMounts, e.restartPolicy)
}

func TestRemoveWithOptionalImageAndMissingContainerConfig(t *testing.T) {
	engine := &updateTestEngine{originalName: "fixture", originalRunning: true, failure: "incomplete"}
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	t.Setenv("DOCKER_HOST", server.URL)
	controller := NewDockerController()
	t.Cleanup(func() { _ = controller.Close() })

	if err := controller.Remove(updateOriginalID, true); err != nil {
		t.Fatalf("container removal failed: %v", err)
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if !engine.originalRemoved {
		t.Fatal("incomplete inspect blocked the container removal")
	}
}
