package generator

// MCPServiceOpts is the language-neutral view of MCPServiceOptions for templates.
type MCPServiceOpts struct {
	App       *MCPAppOpts
	Resources []MCPResourceOpts
}

// MCPMethodOpts is the language-neutral view of per-RPC MCP options for templates.
type MCPMethodOpts struct {
	ToolName        string
	ToolDescription string
	// Progress reports whether (mcp.v1.tool).progress was set. It is advisory:
	// the streaming bridge is emitted based on the response message's shape (see
	// DetectStreamProgress), so this only records the author's declared intent.
	Progress    bool
	Prompt      *MCPPromptOpts
	Elicitation *MCPElicitationOpts
}

// MCPAppOpts mirrors MCPApp for templates.
type MCPAppOpts struct {
	Name        string
	Version     string
	Description string
}

// MCPPromptOpts mirrors MCPPrompt for templates.
// Arguments are derived from the proto message referenced by Schema.
type MCPPromptOpts struct {
	Name        string
	Description string
	Schema      string
	// Role is the MCP message role, "user" or "assistant". Never empty:
	// MCP_ROLE_UNSPECIFIED resolves to "user".
	Role      string
	Arguments []MCPPromptArgOpts
}

// MCPPromptArgOpts describes a single prompt argument resolved from a schema message.
type MCPPromptArgOpts struct {
	Name           string
	Description    string
	Required       bool
	Type           string
	EnumValues     []string
	EnumProtoNames []string // kept in sync with SchemaField to allow direct struct type conversion
}

// MCPResourceOpts mirrors MCPResource for templates.
type MCPResourceOpts struct {
	URI         string
	URITemplate string
	Name        string
	Title       string
	Description string
	MimeType    string
	// Size is the resource content size in bytes. HasSize distinguishes an
	// explicit zero from an absent value.
	Size        int64
	HasSize     bool
	Annotations *MCPAnnotationsOpts
	Icons       []MCPIconOpts
}

// MCPAnnotationsOpts mirrors MCPAnnotations for templates.
type MCPAnnotationsOpts struct {
	// Audience holds MCP roles ("user", "assistant") the resource is intended for.
	Audience     []string
	LastModified string
	// Priority ranges 0.0–1.0. HasPriority distinguishes an explicit 0.0
	// ("least important") from an unset field.
	Priority    float64
	HasPriority bool
}

// MCPIconOpts mirrors MCPIcon for templates.
type MCPIconOpts struct {
	Src      string
	MimeType string
	Sizes    []string
	// Theme is "light", "dark", or "" when the icon suits any background.
	Theme string
}

// MCPElicitationOpts mirrors MCPElicitation for templates.
// Fields are derived from the proto message referenced by Schema.
type MCPElicitationOpts struct {
	Message string
	Schema  string
	// Mode is "form" or "url". Never empty: MCP_ELICITATION_MODE_UNSPECIFIED is
	// resolved here so templates never have to re-run the inference.
	Mode          string
	URL           string
	ElicitationID string
	// Required aborts the tool call when the user declines or the client cannot
	// elicit. When false the call proceeds with the LLM-provided arguments.
	Required bool
	Fields   []MCPElicitFieldOpts
}

// IsURLMode reports whether this elicitation directs the user to an external URL
// instead of rendering a form.
func (e *MCPElicitationOpts) IsURLMode() bool { return e != nil && e.Mode == elicitModeURL }

// MCPElicitFieldOpts describes a single elicitation field resolved from a schema message.
type MCPElicitFieldOpts struct {
	Name           string
	Description    string
	Required       bool
	Type           string
	EnumValues     []string // friendly lowercased names shown in the elicitation form
	EnumProtoNames []string // proto enum names, parallel to EnumValues, used for reverse-mapping after elicitation
}
