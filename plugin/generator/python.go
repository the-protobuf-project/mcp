package generator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/the-protobuf-project/mcp/protobuf/mcppb"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

// PyMethodInfo carries Python-specific type identifiers for a single RPC method.
type PyMethodInfo struct {
	PyMethodName      string // snake_case method name
	PyRequestType     string // e.g. todo_pb2.CreateTodoRequest
	PyResponseType    string // e.g. todo_pb2.Todo
	PyStreamChunkType string // for streaming: e.g. counter_service_pb2.CountStreamChunk
	ToolName          string // MCP tool name
	MethodOpts        *MCPMethodOpts
	StreamProgress    *StreamProgressInfo // Non-nil when server-streaming with MCPProgress
}

// ElicitationSchemaConst holds a pre-serialised JSON Schema for a single elicitation message.
// These are emitted as module-level constants (MessageName_ELICITATION_SCHEMA) so that
// streaming handlers can import and reuse them without redefining the schema inline.
type ElicitationSchemaConst struct {
	Name       string // proto message simple name, e.g. StreamOnboardingValidationArgs
	SchemaJSON string // JSON-encoded {"type":"object","properties":{...},"required":[...]}
}

// PyTplParams is the top-level data fed into the Python code template.
type PyTplParams struct {
	Version            string
	SourcePath         string
	PBImports          string              // import lines for *_pb2 modules
	SchemaJSON         map[string]string   // key: ServiceName_MethodName -> schema JSON
	ToolMeta           map[string]ToolMeta // key: ServiceName_MethodName
	Services           map[string]map[string]PyMethodInfo
	ServiceBasePaths   map[string]string          // key: ServiceName -> default base path
	ServiceOpts        map[string]*MCPServiceOpts // key: ServiceName
	ElicitationSchemas []ElicitationSchemaConst   // sorted by Name, deduplicated
}

// PythonFileGenerator produces a single *_pb2_mcp.py file from a protobuf file.
type PythonFileGenerator struct {
	f   *protogen.File
	gen *protogen.Plugin
}

// NewPythonFileGenerator creates a PythonFileGenerator for the given protobuf file.
func NewPythonFileGenerator(f *protogen.File, gen *protogen.Plugin) *PythonFileGenerator {
	gen.SupportedFeatures |= uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
	return &PythonFileGenerator{f: f, gen: gen}
}

// Generate produces the *_pb2_mcp.py output file.
func (g *PythonFileGenerator) Generate() {
	file := g.f
	hasServices := len(file.Services) > 0
	hasElicit := hasElicitMessages(file.Messages)
	if !hasServices && !hasElicit {
		return
	}

	// Output file path: same directory as the proto, with _pb2_mcp.py suffix.
	// e.g. store/apps/todo/v1/todo_service_pb2_mcp.py
	outName := file.GeneratedFilenamePrefix + "_pb2_mcp.py"
	gf := g.gen.NewGeneratedFile(outName, "")

	params := g.buildPyParams()
	if err := pythonTemplate.Execute(gf, params); err != nil {
		g.gen.Error(err)
	}
}

// buildPyParams iterates over all services/methods and builds the Python template data.
func (g *PythonFileGenerator) buildPyParams() PyTplParams {
	services := make(map[string]map[string]PyMethodInfo)
	schemaJSON := make(map[string]string)
	toolMeta := make(map[string]ToolMeta)
	serviceBasePaths := make(map[string]string)
	serviceOpts := make(map[string]*MCPServiceOpts)

	// Collect all imported proto files needed for request/response types.
	pbImports := make(map[string]bool)

	for _, svc := range g.f.Services {
		methods := make(map[string]PyMethodInfo)

		for _, meth := range svc.Methods {
			if meth.Desc.IsStreamingClient() {
				continue
			}
			streamProgress := DetectProgressStream(meth, func(ident protogen.GoIdent) string {
				// resolveType for Go; Python uses ResultMessage
				return string(ident.GoName)
			})
			if meth.Desc.IsStreamingServer() && streamProgress == nil {
				continue // Server-streaming without MCPProgress convention is not supported
			}

			key := string(svc.Desc.Name()) + "_" + meth.GoName
			toolName := BuildToolName(string(meth.Desc.FullName()))

			// Apply method-level option overrides.
			methOpts := ExtractMethodOptions(meth)
			if methOpts != nil {
				if methOpts.ToolName != "" {
					toolName = methOpts.ToolName
				}
				// Resolve prompt schema → populate Arguments from proto message fields.
				if methOpts.Prompt != nil && methOpts.Prompt.Schema != "" {
					for _, sf := range ResolveSchemaFields(g.gen, methOpts.Prompt.Schema) {
						methOpts.Prompt.Arguments = append(methOpts.Prompt.Arguments, MCPPromptArgOpts(sf))
					}
				}
				// Resolve elicitation schema → populate Fields from proto message fields.
				if methOpts.Elicitation != nil && methOpts.Elicitation.Schema != "" {
					for _, sf := range ResolveSchemaFields(g.gen, methOpts.Elicitation.Schema) {
						methOpts.Elicitation.Fields = append(methOpts.Elicitation.Fields, MCPElicitFieldOpts(sf))
					}
				}
			}

			toolDesc := CleanComment(string(meth.Comments.Leading))
			if methOpts != nil && methOpts.ToolDescription != "" {
				toolDesc = methOpts.ToolDescription
			}

			// Standard schema (root description = tool description, per MCP inputSchema convention)
			stdSchema := messageSchema(meth.Input.Desc, false, toolDesc)
			stdBytes, err := json.Marshal(stdSchema)
			if err != nil {
				panic(fmt.Sprintf("marshal standard schema: %v", err))
			}
			schemaJSON[key] = string(stdBytes)

			toolMeta[key] = ToolMeta{
				Name:        toolName,
				Description: toolDesc,
			}

			// Build Python import paths and type references.
			reqModule := protoPyModule(meth.Input)
			respModule := protoPyModule(meth.Output)
			pbImports[reqModule] = true
			pbImports[respModule] = true

			pyReqType := protoPyType(meth.Input)
			pyRespType := protoPyType(meth.Output)
			pyStreamChunkType := ""
			if streamProgress != nil && streamProgress.ResultMessage != nil {
				pyRespType = protoPyType(streamProgress.ResultMessage)
				pyStreamChunkType = protoPyType(meth.Output)
				pbImports[protoPyModule(streamProgress.ResultMessage)] = true
			}

			methods[meth.GoName] = PyMethodInfo{
				PyMethodName:      toSnakeCase(meth.GoName),
				PyRequestType:     pyReqType,
				PyResponseType:    pyRespType,
				PyStreamChunkType: pyStreamChunkType,
				ToolName:          toolName,
				MethodOpts:        methOpts,
				StreamProgress:    streamProgress,
			}
		}

		svcName := string(svc.Desc.Name())
		services[svcName] = methods
		serviceBasePaths[svcName] = "/" + strings.ToLower(strings.ReplaceAll(string(svc.Desc.FullName()), ".", "/")) + "/mcp"
		svcOpt := ExtractServiceOptions(svc)
		apiResources := ExtractGoogleAPIResources(svc)
		if len(apiResources) > 0 {
			if svcOpt == nil {
				svcOpt = &MCPServiceOpts{}
			}
			svcOpt.Resources = apiResources
		}
		serviceOpts[svcName] = svcOpt
	}

	// Build import lines.
	var importLines []string
	for mod := range pbImports {
		importLines = append(importLines, fmt.Sprintf("import %s", mod))
	}
	sort.Strings(importLines)

	// Add PyDescription to ToolMeta (Python-safe string literal).
	for key, meta := range toolMeta {
		meta.Description = strings.TrimSpace(meta.Description)
		toolMeta[key] = meta
	}

	// Collect elicitation schema constants from all messages in this file
	// that carry at least one (mcp.field) annotation.
	elicitSeen := make(map[string]bool)
	elicitSchemas := collectElicitSchemas(g.gen, g.f.Messages, elicitSeen)
	sort.Slice(elicitSchemas, func(i, j int) bool {
		return elicitSchemas[i].Name < elicitSchemas[j].Name
	})

	return PyTplParams{
		Version:            PluginVersion,
		SourcePath:         g.f.Desc.Path(),
		PBImports:          strings.Join(importLines, "\n"),
		SchemaJSON:         schemaJSON,
		ToolMeta:           toolMeta,
		Services:           services,
		ServiceBasePaths:   serviceBasePaths,
		ServiceOpts:        serviceOpts,
		ElicitationSchemas: elicitSchemas,
	}
}

// hasElicitMessages reports whether any message in msgs (recursively) has at least one
// field annotated with (mcp.field).
func hasElicitMessages(msgs []*protogen.Message) bool {
	for _, m := range msgs {
		for _, f := range m.Fields {
			if proto.HasExtension(f.Desc.Options(), mcppb.E_Field) {
				return true
			}
		}
		if hasElicitMessages(m.Messages) {
			return true
		}
	}
	return false
}

// collectElicitSchemas recursively walks msgs and returns an ElicitationSchemaConst for every
// message that has at least one (mcp.field) annotated field.
// seen is used to deduplicate by fully-qualified message name.
func collectElicitSchemas(gen *protogen.Plugin, msgs []*protogen.Message, seen map[string]bool) []ElicitationSchemaConst {
	var result []ElicitationSchemaConst
	for _, m := range msgs {
		fqn := string(m.Desc.FullName())
		if !seen[fqn] {
			hasMCP := false
			for _, f := range m.Fields {
				if proto.HasExtension(f.Desc.Options(), mcppb.E_Field) {
					hasMCP = true
					break
				}
			}
			if hasMCP {
				seen[fqn] = true
				fields := ResolveSchemaFields(gen, fqn)
				if len(fields) > 0 {
					result = append(result, ElicitationSchemaConst{
						Name:       string(m.Desc.Name()),
						SchemaJSON: buildElicitSchemaJSON(fields),
					})
				}
			}
		}
		result = append(result, collectElicitSchemas(gen, m.Messages, seen)...)
	}
	return result
}

// buildElicitSchemaJSON serialises a slice of SchemaFields into a JSON Schema object string.
func buildElicitSchemaJSON(fields []SchemaField) string {
	type prop struct {
		Type        string   `json:"type"`
		Description string   `json:"description,omitempty"`
		Enum        []string `json:"enum,omitempty"`
	}
	props := make(map[string]prop, len(fields))
	var required []string
	for _, f := range fields {
		p := prop{Type: f.Type, Description: f.Description}
		if len(f.EnumValues) > 0 {
			p.Enum = f.EnumValues
		}
		props[f.Name] = p
		if f.Required {
			required = append(required, f.Name)
		}
	}
	schema := struct {
		Type       string          `json:"type"`
		Properties map[string]prop `json:"properties"`
		Required   []string        `json:"required,omitempty"`
	}{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return `{"type":"object","properties":{}}`
	}
	return string(b)
}
