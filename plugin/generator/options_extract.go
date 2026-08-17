package generator

import (
	mcppb "buf.build/gen/go/the-protobuf-project/mcp/protocolbuffers/go/mcp/protobuf"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

// ExtractServiceOptions reads the mcp.service extension from a service descriptor.
func ExtractServiceOptions(svc *protogen.Service) *MCPServiceOpts {
	opts := svc.Desc.Options()
	if opts == nil {
		return nil
	}
	ext, ok := proto.GetExtension(opts, mcppb.E_Service).(*mcppb.MCPServiceOptions)
	if !ok || ext == nil {
		return nil
	}
	result := &MCPServiceOpts{}
	if ext.App != nil {
		result.App = &MCPAppOpts{
			Name:        ext.App.GetName(),
			Version:     ext.App.GetVersion(),
			Description: ext.App.GetDescription(),
		}
	}
	return result
}

// ExtractMethodOptions reads mcp.tool, mcp.prompt, and mcp.elicitation
// extensions from a method descriptor and merges them into a single MCPMethodOpts.
func ExtractMethodOptions(meth *protogen.Method) *MCPMethodOpts {
	opts := meth.Desc.Options()
	if opts == nil {
		return nil
	}

	result := &MCPMethodOpts{}
	hasAnything := false

	// mcp.tool — name/description overrides
	toolExt, ok := proto.GetExtension(opts, mcppb.E_Tool).(*mcppb.MCPToolOptions)
	if ok && toolExt != nil {
		result.ToolName = toolExt.GetName()
		result.ToolDescription = toolExt.GetDescription()
		hasAnything = true
	}

	// mcp.prompt — per-RPC prompt template with schema reference
	promptExt, ok := proto.GetExtension(opts, mcppb.E_Prompt).(*mcppb.MCPPrompt)
	if ok && promptExt != nil {
		result.Prompt = &MCPPromptOpts{
			Name:        promptExt.GetName(),
			Description: promptExt.GetDescription(),
			Schema:      promptExt.GetSchema(),
		}
		hasAnything = true
	}

	// mcp.elicitation — confirmation dialog with schema reference
	elicitExt, ok := proto.GetExtension(opts, mcppb.E_Elicitation).(*mcppb.MCPElicitation)
	if ok && elicitExt != nil {
		result.Elicitation = &MCPElicitationOpts{
			Message: elicitExt.GetMessage(),
			Schema:  elicitExt.GetSchema(),
		}
		hasAnything = true
	}

	if !hasAnything {
		return nil
	}
	return result
}
