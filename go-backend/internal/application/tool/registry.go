package tool

import (
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/spacexc/go-backend/internal/domain/tool"
	"github.com/spacexc/go-backend/internal/infrastructure/llm"
)

// ToolRegistry implements domain.ToolRegistry
type ToolRegistry struct {
	tools  map[string]tool.Tool
	mu     sync.RWMutex
	logger *logrus.Logger
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry(logger *logrus.Logger) *ToolRegistry {
	return &ToolRegistry{
		tools:  make(map[string]tool.Tool),
		logger: logger,
	}
}

// Register adds a tool to the registry
func (r *ToolRegistry) Register(t tool.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
	r.logger.WithField("tool", t.Name()).Debug("tool registered")
}

// Get retrieves a tool by name
func (r *ToolRegistry) Get(name string) (tool.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools
func (r *ToolRegistry) List() []tool.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]tool.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// GetToolDefs returns OpenAI-format tool definitions for all registered tools
func (r *ToolRegistry) GetToolDefs() []llm.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]llm.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}