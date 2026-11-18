package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
)

// SetCommunityAppsCatalog wires the host catalog before serving requests.
func (s *Server) SetCommunityAppsCatalog(catalog *communityapps.Catalog) {
	s.communityAppsCatalog = catalog
}

type communityAppsSearchArgs struct {
	Query string `json:"query,omitempty" jsonschema:"Search term (maximum 256 bytes)"`
}

type communityAppInspectionArgs struct {
	Reference string `json:"reference" jsonschema:"Listing reference returned by search_community_applications"`
}

func (s *Server) registerCommunityAppsTools() {
	addToolDirect(s, &mcp.Tool{
		Name:        "search_community_applications",
		Description: "Search Docker listings in the host's filtered Community Applications catalog.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args communityAppsSearchArgs) (*mcp.CallToolResult, any, error) {
		result, err := s.communityAppsCatalog.Search(ctx, args.Query)
		if err != nil {
			return catalogToolError(err)
		}
		return jsonResult(result)
	})
	addToolDirect(s, &mcp.Tool{
		Name:        "inspect_community_application",
		Description: "Inspect the exact listing's editable Docker template fields and known eligibility issues. Environment values are redacted.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args communityAppInspectionArgs) (*mcp.CallToolResult, any, error) {
		result, err := s.communityAppsCatalog.Inspect(ctx, args.Reference)
		if err != nil {
			return catalogToolError(err)
		}
		return jsonResult(result)
	})
}

func catalogToolError(err error) (*mcp.CallToolResult, any, error) {
	result, data, marshalErr := jsonResult(communityapps.ProblemForError(err))
	result.IsError = true
	return result, data, marshalErr
}
