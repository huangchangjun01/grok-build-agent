package tool

// Tool defines the interface that all tools must implement.
type Tool interface {
	// Name returns the unique name of the tool.
	Name() string

	// Description returns a human-readable description of the tool.
	Description() string

	// Parameters returns the JSON Schema describing the tool's input parameters.
	Parameters() map[string]interface{}

	// Execute runs the tool with the given context and returns the result.
	Execute(ctx ToolContext) (ToolResult, error)
}

// ToolContext provides the execution context for a tool invocation.
type ToolContext struct {
	SessionID    string
	WorkDir      string
	Params       map[string]interface{}
	UserApproval func(prompt string) bool
}

// ToolResult represents the outcome of a tool execution.
type ToolResult struct {
	Success bool                   `json:"success"`
	Output  string                 `json:"output"`
	Error   string                 `json:"error,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// ToolRegistry manages the registration and lookup of available tools.
type ToolRegistry interface {
	// Register adds a tool to the registry.
	Register(tool Tool)

	// Get retrieves a tool by name.
	Get(name string) (Tool, bool)

	// List returns all registered tools.
	List() []Tool
}