package mcp

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestCommunityAppsMCPRemainsReadableWithWritesLocked(t *testing.T) {
	server := NewServer(&domain.Context{ReadOnly: true}, newMockCacheProvider())
	server.SetNetworkWriteLock(true)
	server.SetCommunityAppsCatalog(communityapps.NewCatalog(os.DirFS(communityappstest.CopyHost(t))))
	if err := server.Initialize(); err != nil {
		t.Fatal(err)
	}
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"search_community_applications", "inspect_community_application"} {
		if !slices.ContainsFunc(tools.Tools, func(tool *mcp.Tool) bool {
			return tool.Name == name && tool.Annotations != nil && tool.Annotations.ReadOnlyHint
		}) {
			t.Fatalf("%s is unavailable or not read-only", name)
		}
	}
	result, content := callToolJSON(t, cs, "search_community_applications", map[string]any{"query": "fixture"})
	var response communityapps.SearchResult
	if err := json.Unmarshal([]byte(content), &response); err != nil || result.IsError || len(response.Listings) != 2 {
		t.Fatalf("search failed: %s", content)
	}
	result, content = callToolJSON(t, cs, "inspect_community_application", map[string]any{"reference": response.Listings[1].Reference})
	var inspection communityapps.Inspection
	if err := json.Unmarshal([]byte(content), &inspection); err != nil || result.IsError || inspection.Listing.Variant != "edge" {
		t.Fatalf("inspection failed: %s", content)
	}
	if strings.Contains(content, "sentinel") {
		t.Fatal("inspection disclosed a sensitive value")
	}
	result, content = callToolJSON(t, cs, "inspect_community_application", map[string]any{"reference": strings.Repeat("a", 64)})
	var problem communityapps.Problem
	if err := json.Unmarshal([]byte(content), &problem); err != nil || !result.IsError || problem.Code != "listing_unavailable" {
		t.Fatalf("missing listing did not produce an actionable tool error: %s", content)
	}
}
