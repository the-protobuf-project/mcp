package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/the-protobuf-project/grpc-mcp-gateway/mcp/protobuf/mcppb"
)

// SendProgressFromProto sends an MCP progress notification from an MCPProgress proto.
// If token or p is nil, it returns nil. Used by generated streaming tool handlers.
func SendProgressFromProto(ctx context.Context, session *mcp.ServerSession, token any, p *mcppb.MCPProgress) error {
	if token == nil || p == nil || session == nil {
		return nil
	}
	params := &mcp.ProgressNotificationParams{
		ProgressToken: token,
		Progress:      p.Progress,
		Message:       p.Message,
	}
	if p.Total != nil {
		params.Total = *p.Total
	}
	return session.NotifyProgress(ctx, params)
}

// SendDoneProgress sends a final MCP progress notification (progress=1, total=1)
// with resultJSON as the message, signaling to the MCP client that the streaming
// operation has completed. Generated non-blocking streaming handlers call this
// when the result chunk arrives from the gRPC server method.
func SendDoneProgress(ctx context.Context, session *mcp.ServerSession, token any, resultJSON string) error {
	if token == nil || session == nil {
		return nil
	}
	one := 1.0
	return SendProgressFromProto(ctx, session, token, &mcppb.MCPProgress{
		Progress: 1.0,
		Total:    &one,
		Message:  resultJSON,
	})
}

// DefaultPromptHandler returns a prompt handler that produces a single user
// message containing the prompt description. It is used as a placeholder for
// prompts declared via MCP proto options. Replace it by calling
// server.RemovePrompts / server.AddPrompt with your own handler.
func DefaultPromptHandler(description string) func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: description,
			Messages: []*mcp.PromptMessage{
				{
					Role:    "user",
					Content: &mcp.TextContent{Text: description},
				},
			},
		}, nil
	}
}

// DefaultResourceHandler returns a resource handler that returns an empty JSON
// object. It is used as a placeholder for resources declared via MCP proto
// options. Replace it by calling server.RemoveResources / server.AddResource
// with your own handler.
func DefaultResourceHandler() func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, Text: "{}"},
			},
		}, nil
	}
}

// AppResourceURI returns the canonical ui:// resource URI for a service app.
func AppResourceURI(serviceName string) string {
	return fmt.Sprintf("ui://%s/app.html", strings.ToLower(serviceName))
}

// SetToolAppMeta returns a shallow clone of tool with _meta.ui.resourceUri set,
// which makes the tool show up as an MCP App in supporting hosts.
func SetToolAppMeta(tool *mcp.Tool, resourceURI string) *mcp.Tool {
	cloned := *tool
	cloned.Meta = mcp.Meta{
		"ui": map[string]any{
			"resourceUri": resourceURI,
		},
	}
	return &cloned
}

// DefaultAppHTML returns a minimal HTML page for an MCP App placeholder.
func DefaultAppHTML(appName, version, description string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>%s</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 600px; margin: 40px auto; padding: 0 20px; color: #333; }
  h1 { font-size: 1.5rem; } p { color: #666; } .version { font-size: 0.85rem; color: #999; }
</style>
</head>
<body>
  <h1>%s</h1>
  <p class="version">v%s</p>
  <p>%s</p>
  <p>This is a generated MCP App placeholder. Replace this resource with your own UI.</p>
</body>
</html>`, appName, appName, version, description)
}

// DefaultAppResourceHandler returns a resource handler that serves the default
// app HTML page. The returned handler is suitable for registration with
// server.AddResource for the ui:// resource URI.
func DefaultAppResourceHandler(appName, version, description string) func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	html := DefaultAppHTML(appName, version, description)
	return func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: "text/html", Text: html},
			},
		}, nil
	}
}

// CompletionHandlerFromEnums builds a CompletionHandler that serves autocomplete
// values for prompt arguments. The enumValues map is keyed by "promptName:argName".
func CompletionHandlerFromEnums(enumValues map[string][]string) func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	return func(_ context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
		if req.Params.Ref.Type != "ref/prompt" {
			return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{}}}, nil
		}
		key := req.Params.Ref.Name + ":" + req.Params.Argument.Name
		values, ok := enumValues[key]
		if !ok {
			return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{}}}, nil
		}
		prefix := strings.ToLower(req.Params.Argument.Value)
		filtered := []string{}
		for _, v := range values {
			if strings.HasPrefix(strings.ToLower(v), prefix) {
				filtered = append(filtered, v)
			}
		}
		return &mcp.CompleteResult{
			Completion: mcp.CompletionResultDetails{
				Values:  filtered,
				Total:   len(filtered),
				HasMore: false,
			},
		}, nil
	}
}

// ElicitField describes a field for an elicitation (confirmation) request.
// Used with RunElicitation to build a form shown to the user before tool execution.
type ElicitField struct {
	Name        string   // JSON property name
	Description string   // Shown in the form
	Required    bool     // If true, user must provide a value
	Type        string   // JSON Schema type: "string", "number", "boolean"
	EnumValues  []string // Optional: friendly names shown in the elicitation form
	ProtoValues []string // Optional: proto enum names, parallel to EnumValues, for reverse-mapping after accept
}

// MergeElicitResult overlays the accepted elicitation result content onto the
// original LLM tool args JSON. Enum fields whose ElicitField has ProtoValues
// are reverse-mapped from their friendly UI names back to their protobuf enum
// names so that protojson.Unmarshal decodes them correctly. The returned bytes
// are always valid JSON.
func MergeElicitResult(args json.RawMessage, content map[string]any, fields []ElicitField) json.RawMessage {
	if len(content) == 0 {
		return args
	}
	// Build friendly-name → proto-name lookup per field.
	protoMap := make(map[string]map[string]string, len(fields))
	for _, f := range fields {
		if len(f.ProtoValues) > 0 && len(f.ProtoValues) == len(f.EnumValues) {
			m := make(map[string]string, len(f.EnumValues))
			for i, friendly := range f.EnumValues {
				m[friendly] = f.ProtoValues[i]
			}
			protoMap[f.Name] = m
		}
	}
	// Unmarshal existing args.
	var merged map[string]any
	if err := json.Unmarshal(args, &merged); err != nil || merged == nil {
		merged = make(map[string]any)
	}
	// Overlay elicitation content, reverse-mapping enum values where needed.
	for k, v := range content {
		if m, ok := protoMap[k]; ok {
			if s, ok := v.(string); ok {
				if proto, ok := m[s]; ok {
					v = proto
				}
			}
		}
		merged[k] = v
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return args
	}
	return out
}

// ElicitRequestID is the input-request ID under which generated tool handlers
// attach their confirmation elicitation to a CallToolResult, and under which
// the client echoes the answer back in CallToolParams.InputResponses.
const ElicitRequestID = "elicitation"

// ElicitSchema builds the JSON Schema an elicitation form is rendered from.
func ElicitSchema(fields []ElicitField) *jsonschema.Schema {
	props := make(map[string]*jsonschema.Schema, len(fields))
	var required []string
	for _, f := range fields {
		sch := &jsonschema.Schema{Type: f.Type, Description: f.Description}
		if len(f.EnumValues) > 0 {
			for _, v := range f.EnumValues {
				sch.Enum = append(sch.Enum, v)
			}
		}
		props[f.Name] = sch
		if f.Required {
			required = append(required, f.Name)
		}
	}
	return &jsonschema.Schema{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
}

// RunElicitation advances the multi round-trip elicitation handshake
// (SEP-2322) for one tool call, building a form from the given fields.
//
// Protocol version 2026-07-28 forbids a server from issuing an elicitation
// request while it is serving a request. A handler instead returns a result
// carrying an InputRequests map; the client fulfills it and retries the same
// tool call with the answers in Params.InputResponses, so the handler runs
// twice. Clients on older protocol versions never see this: the go-sdk's
// server middleware performs the round trip on their behalf and reinvokes the
// handler, so a handler only needs the one code path.
//
// When err is nil, exactly one of the first two results is non-nil:
//   - result: the client answered. Inspect result.Action; anything other than
//     "accept" means the user declined.
//   - pending: no answer yet. Return it from the handler unchanged to ask the
//     client for input.
func RunElicitation(req *mcp.CallToolRequest, message string, fields []ElicitField) (result *mcp.ElicitResult, pending *mcp.CallToolResult, err error) {
	if req == nil || req.Params == nil {
		return nil, nil, fmt.Errorf("elicitation %q: no tool call request", message)
	}
	if resp, ok := req.Params.InputResponses[ElicitRequestID]; ok {
		res, ok := resp.(*mcp.ElicitResult)
		if !ok {
			return nil, nil, fmt.Errorf("elicitation %q: input response is %T, want *mcp.ElicitResult", message, resp)
		}
		return res, nil, nil
	}
	return nil, &mcp.CallToolResult{
		InputRequests: mcp.InputRequestMap{
			ElicitRequestID: &mcp.ElicitParams{
				Message:         message,
				RequestedSchema: ElicitSchema(fields),
			},
		},
	}, nil
}
