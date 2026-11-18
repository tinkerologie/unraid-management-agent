package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/api"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestGetContainerInfoUsesLiveDockerIdentityAndNativeTemplate(t *testing.T) {
	const link = "https://example.com/templates/heimdall.xml"
	communityappstest.UseDockerInspectEngine(t, map[string]string{
		"heimdall": `{"Id":"2222222222220001","Name":"/heimdall","State":{"Status":"running"},"Config":{"Image":"lscr.io/linuxserver/heimdall","Labels":{"net.unraid.docker.managed":"dockerman"}}}`,
	})
	root := t.TempDir()
	const filename = "boot/config/plugins/dockerMan/templates-user/my-heimdall.xml"
	communityappstest.WriteFile(t, root, filename, "<Container><Name>heimdall</Name><TemplateURL>"+link+"</TemplateURL></Container>")
	ctx := &domain.Context{ReadOnly: true}
	apiServer := api.NewServer(ctx)
	apiServer.SetCommunityAppsService(communityapps.NewService(nil, communityapps.ServiceOptions{HostRoot: root}))
	server := NewServer(ctx, apiServer)
	if err := server.Initialize(); err != nil {
		t.Fatal(err)
	}
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, current := callToolJSON(t, cs, "get_container_info", map[string]any{"container_id": "heimdall"})
	var detail dto.ContainerDetail
	if err := json.Unmarshal([]byte(current), &detail); err != nil || detail.ID != "222222222222" || detail.CatalogLink != link {
		t.Fatalf("MCP did not return the current identity and origin: %s", current)
	}

	communityappstest.WriteFile(t, root, filename, "<Container><Name>heimdall</Name></Container>")
	_, unknown := callToolJSON(t, cs, "get_container_info", map[string]any{"container_id": "heimdall"})
	if !strings.Contains(unknown, "222222222222") || strings.Contains(unknown, "catalog_link") {
		t.Fatalf("MCP did not report unknown origin after the template link was removed: %s", unknown)
	}

	_, absent := callToolJSON(t, cs, "get_container_info", map[string]any{"container_id": "111111111111"})
	if !strings.Contains(absent, "not found") {
		t.Fatalf("MCP did not report the former ID as missing: %s", absent)
	}
}
