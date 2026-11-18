package mcp

import (
	"bytes"
	"context"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/api"
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
	addCommunityAppsWriteTool(s, &mcp.Tool{
		Name:        "prepare_community_app_install",
		Description: "Prepare and review a specific Community Applications Docker install. Environment values are redacted and the plan expires in 15 minutes.",
	}, func(ctx context.Context, args communityAppsPrepareArgs) (*mcp.CallToolResult, any, error) {
		result, err := s.communityAppsService.Prepare(ctx, args.PrepareRequest)
		if err != nil {
			return catalogToolError(err)
		}
		return jsonResult(result)
	})
	addCommunityAppsWriteTool(s, &mcp.Tool{
		Name:        "confirm_community_app_install",
		Description: "Install the exact reviewed Community Applications plan. Obtain operator approval for the preview and send its approval token. Repeating the same approval returns the original attempt.",
	}, func(ctx context.Context, args communityAppsConfirmArgs) (*mcp.CallToolResult, any, error) {
		attempt, err := s.communityAppsService.Confirm(ctx, args.PlanID, args.ApprovalToken)
		if err != nil {
			return catalogToolError(err)
		}
		return jsonResult(attempt)
	})
	addToolDirect(s, &mcp.Tool{
		Name:        "get_community_app_install_attempt",
		Description: "Read redacted progress or the final result of a confirmed Community Applications install.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, args communityAppsAttemptArgs) (*mcp.CallToolResult, any, error) {
		attempt, err := s.communityAppsService.GetAttempt(args.AttemptID)
		if err != nil {
			return catalogToolError(err)
		}
		return jsonResult(attempt)
	})
}

func catalogToolError(err error) (*mcp.CallToolResult, any, error) {
	result, data, marshalErr := jsonResult(communityapps.ProblemForError(err))
	result.IsError = true
	return result, data, marshalErr
}

// SetCommunityAppsService wires the install service shared by REST and MCP.
func (s *Server) SetCommunityAppsService(service *communityapps.Service) {
	s.communityAppsService = service
}

type communityAppsPrepareArgs struct {
	communityapps.PrepareRequest
	Confirm bool `json:"confirm,omitempty" jsonschema:"Required when the MCP tool access policy is ask"`
}

type communityAppsConfirmArgs struct {
	communityapps.ConfirmRequest
	PlanID  string `json:"plan_id" jsonschema:"ID of the exact plan from prepare_community_app_install"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"Required when the MCP tool access policy is ask"`
}

type communityAppsAttemptArgs struct {
	AttemptID string `json:"attempt_id" jsonschema:"Attempt ID from confirm_community_app_install"`
}

func requireCommunityAppsCredential(req *mcp.CallToolRequest) error {
	if req.Extra == nil || req.Extra.Header == nil {
		return &communityapps.Problem{Code: "credential_required", Message: "Use Streamable HTTP MCP with a configured bearer or connect secret for Community Applications installs."}
	}
	if !api.HasMCPRequestCredential(req.Extra.TokenInfo) {
		return communityapps.ErrPlanCredential
	}
	return nil
}

// addCommunityAppsWriteTool applies ordinary write policy and mandatory per-
// message credentials before decoding sensitive arguments. The low-level SDK
// registration leaves error formatting under our control.
func addCommunityAppsWriteTool[Args any](s *Server, tool *mcp.Tool, handler func(context.Context, Args) (*mcp.CallToolResult, any, error)) {
	schema, err := jsonschema.For[Args](nil)
	if err != nil {
		panic(err)
	}
	tool.InputSchema = schema
	if s.toolPolicyStore != nil {
		destructive := tool.Annotations != nil && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
		s.toolPolicyStore.RegisterTool(tool.Name, tool.Description, false, destructive)
	}
	s.mcpServer.AddTool(tool, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		policy := s.effectivePolicy(tool.Name)
		if policy == domain.PolicyReadOnly {
			return textResult(s.blockedMessage(tool.Name)), nil
		}
		if err := requireCommunityAppsCredential(request); err != nil {
			result, _, err := catalogToolError(err)
			return result, err
		}
		args, err := communityapps.DecodePlanArguments[Args](bytes.NewReader(request.Params.Arguments))
		if err != nil {
			result, _, err := catalogToolError(err)
			return result, err
		}
		if policy == domain.PolicyAsk && !isConfirmed(request.Params.Arguments, &args) {
			return textResult("Confirmation required by the MCP tool access policy. Set confirm=true after operator approval."), nil
		}
		result, _, err := handler(ctx, args)
		return result, err
	})
}
