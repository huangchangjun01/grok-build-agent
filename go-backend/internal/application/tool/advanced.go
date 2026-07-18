package tool

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/spacexc/go-backend/internal/domain/tool"
	"github.com/spacexc/go-backend/internal/infrastructure/shell"
)

// =============================================================================
// EnterPlanModeTool
// =============================================================================

// EnterPlanModeTool enters plan mode.
type EnterPlanModeTool struct {
	logger      *logrus.Logger
	planModeSet func(sessionID string, enabled bool) error
}

// NewEnterPlanModeTool creates a new EnterPlanModeTool.
func NewEnterPlanModeTool(logger *logrus.Logger, planModeSet func(sessionID string, enabled bool) error) *EnterPlanModeTool {
	return &EnterPlanModeTool{
		logger:      logger,
		planModeSet: planModeSet,
	}
}

func (t *EnterPlanModeTool) Name() string {
	return "enter_plan_mode"
}

func (t *EnterPlanModeTool) Description() string {
	return "Enters plan mode. In plan mode, the agent will create a detailed plan before executing any actions. Use this when you need to think through a complex task before making changes."
}

func (t *EnterPlanModeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "The reason for entering plan mode.",
			},
		},
	}
}

func (t *EnterPlanModeTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	reason := ""
	if r, ok := ctx.Params["reason"].(string); ok {
		reason = r
	}

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"reason":     reason,
	}).Info("entering plan mode")

	if err := t.planModeSet(ctx.SessionID, true); err != nil {
		t.logger.WithError(err).Error("failed to enter plan mode")
		return tool.ToolResult{
			Success: false,
			Output:  "Failed to enter plan mode.",
			Error:   err.Error(),
		}, nil
	}

	return tool.ToolResult{
		Success: true,
		Output:  "Plan mode enabled. The agent will now create a detailed plan before executing actions.",
	}, nil
}

// =============================================================================
// ExitPlanModeTool
// =============================================================================

// ExitPlanModeTool exits plan mode.
type ExitPlanModeTool struct {
	logger      *logrus.Logger
	planModeSet func(sessionID string, enabled bool) error
}

// NewExitPlanModeTool creates a new ExitPlanModeTool.
func NewExitPlanModeTool(logger *logrus.Logger, planModeSet func(sessionID string, enabled bool) error) *ExitPlanModeTool {
	return &ExitPlanModeTool{
		logger:      logger,
		planModeSet: planModeSet,
	}
}

func (t *ExitPlanModeTool) Name() string {
	return "exit_plan_mode"
}

func (t *ExitPlanModeTool) Description() string {
	return "Exits plan mode and returns to normal execution mode. Use this after the plan has been approved and you are ready to execute the planned actions."
}

func (t *ExitPlanModeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"plan_summary": map[string]interface{}{
				"type":        "string",
				"description": "A brief summary of the plan that was created.",
			},
		},
	}
}

func (t *ExitPlanModeTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	planSummary := ""
	if s, ok := ctx.Params["plan_summary"].(string); ok {
		planSummary = s
	}

	t.logger.WithFields(logrus.Fields{
		"session_id":   ctx.SessionID,
		"plan_summary": planSummary,
	}).Info("exiting plan mode")

	if err := t.planModeSet(ctx.SessionID, false); err != nil {
		t.logger.WithError(err).Error("failed to exit plan mode")
		return tool.ToolResult{
			Success: false,
			Output:  "Failed to exit plan mode.",
			Error:   err.Error(),
		}, nil
	}

	return tool.ToolResult{
		Success: true,
		Output:  "Plan mode disabled. Returning to normal execution mode.",
	}, nil
}

// =============================================================================
// UpdateGoalTool
// =============================================================================

// UpdateGoalTool updates the agent's goal.
type UpdateGoalTool struct {
	logger *logrus.Logger
	goalSet func(sessionID string, goal string) error
}

// NewUpdateGoalTool creates a new UpdateGoalTool.
func NewUpdateGoalTool(logger *logrus.Logger, goalSet func(sessionID string, goal string) error) *UpdateGoalTool {
	return &UpdateGoalTool{
		logger:  logger,
		goalSet: goalSet,
	}
}

func (t *UpdateGoalTool) Name() string {
	return "update_goal"
}

func (t *UpdateGoalTool) Description() string {
	return "Updates the agent's current goal. Use this to set or change the overall objective the agent is working towards."
}

func (t *UpdateGoalTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"goal": map[string]interface{}{
				"type":        "string",
				"description": "The new goal description.",
			},
		},
		"required": []string{"goal"},
	}
}

func (t *UpdateGoalTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	goal, ok := ctx.Params["goal"].(string)
	if !ok || goal == "" {
		return tool.ToolResult{
			Success: false,
			Output:  "No goal provided.",
			Error:   "Missing required parameter: goal",
		}, nil
	}

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"goal":       goal,
	}).Info("updating agent goal")

	if err := t.goalSet(ctx.SessionID, goal); err != nil {
		t.logger.WithError(err).Error("failed to update goal")
		return tool.ToolResult{
			Success: false,
			Output:  "Failed to update goal.",
			Error:   err.Error(),
		}, nil
	}

	return tool.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Goal updated to: %s", goal),
	}, nil
}

// =============================================================================
// SchedulerTool
// =============================================================================

// SchedulerTool schedules recurring tasks.
type SchedulerTool struct {
	executor *shell.Executor
	logger   *logrus.Logger
}

// NewSchedulerTool creates a new SchedulerTool.
func NewSchedulerTool(executor *shell.Executor, logger *logrus.Logger) *SchedulerTool {
	return &SchedulerTool{
		executor: executor,
		logger:   logger,
	}
}

func (t *SchedulerTool) Name() string {
	return "scheduler"
}

func (t *SchedulerTool) Description() string {
	return "Schedules a command to run on a recurring basis. Provides a simple cron-like scheduling interface for shell commands."
}

func (t *SchedulerTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to schedule.",
			},
			"schedule": map[string]interface{}{
				"type":        "string",
				"description": "The schedule in cron-like format (e.g., '0 * * * *' for hourly).",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "A human-readable description of the scheduled task.",
			},
		},
		"required": []string{"command", "schedule"},
	}
}

func (t *SchedulerTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	command, ok := ctx.Params["command"].(string)
	if !ok || command == "" {
		return tool.ToolResult{
			Success: false,
			Output:  "No command provided.",
			Error:   "Missing required parameter: command",
		}, nil
	}

	schedule, ok := ctx.Params["schedule"].(string)
	if !ok || schedule == "" {
		return tool.ToolResult{
			Success: false,
			Output:  "No schedule provided.",
			Error:   "Missing required parameter: schedule",
		}, nil
	}

	description := ""
	if d, ok := ctx.Params["description"].(string); ok {
		description = d
	}

	t.logger.WithFields(logrus.Fields{
		"session_id":  ctx.SessionID,
		"command":     command,
		"schedule":    schedule,
		"description": description,
	}).Info("scheduling recurring task")

	// Record the scheduling intent; the actual cron scheduling is handled by the platform layer.
	return tool.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Task scheduled successfully. Command '%s' will run on schedule: %s", command, schedule),
		Data: map[string]interface{}{
			"command":     command,
			"schedule":    schedule,
			"description": description,
		},
	}, nil
}

// =============================================================================
// ImageGenTool
// =============================================================================

// ImageGenTool generates images (placeholder).
type ImageGenTool struct {
	logger *logrus.Logger
}

// NewImageGenTool creates a new ImageGenTool.
func NewImageGenTool(logger *logrus.Logger) *ImageGenTool {
	return &ImageGenTool{logger: logger}
}

func (t *ImageGenTool) Name() string {
	return "image_gen"
}

func (t *ImageGenTool) Description() string {
	return "Generates images from text descriptions. This feature is not yet implemented and will be available in a future update."
}

func (t *ImageGenTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "A text description of the image to generate.",
			},
			"size": map[string]interface{}{
				"type":        "string",
				"description": "The desired image size (e.g., '1024x1024').",
			},
			"style": map[string]interface{}{
				"type":        "string",
				"description": "The visual style of the generated image.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *ImageGenTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	prompt := ""
	if p, ok := ctx.Params["prompt"].(string); ok {
		prompt = p
	}

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"prompt":     prompt,
	}).Info("image generation requested (not yet implemented)")

	return tool.ToolResult{
		Success: true,
		Output:  "Image generation is not yet implemented. This feature is planned for a future release. In the meantime, you can use external image generation services like DALL-E, Midjourney, or Stable Diffusion.",
		Data: map[string]interface{}{
			"status": "not_implemented",
			"prompt": prompt,
		},
	}, nil
}

// =============================================================================
// ImageEditTool
// =============================================================================

// ImageEditTool edits images (placeholder).
type ImageEditTool struct {
	logger *logrus.Logger
}

// NewImageEditTool creates a new ImageEditTool.
func NewImageEditTool(logger *logrus.Logger) *ImageEditTool {
	return &ImageEditTool{logger: logger}
}

func (t *ImageEditTool) Name() string {
	return "image_edit"
}

func (t *ImageEditTool) Description() string {
	return "Edits existing images based on text instructions. This feature is not yet implemented and will be available in a future update."
}

func (t *ImageEditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"image_path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the image file to edit.",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "A description of the desired edits.",
			},
		},
		"required": []string{"image_path", "prompt"},
	}
}

func (t *ImageEditTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	imagePath := ""
	if p, ok := ctx.Params["image_path"].(string); ok {
		imagePath = p
	}
	prompt := ""
	if p, ok := ctx.Params["prompt"].(string); ok {
		prompt = p
	}

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"image_path": imagePath,
		"prompt":     prompt,
	}).Info("image editing requested (not yet implemented)")

	return tool.ToolResult{
		Success: true,
		Output:  "Image editing is not yet implemented. This feature is planned for a future release. You can use external tools like Photoshop, GIMP, or online editors for now.",
		Data: map[string]interface{}{
			"status":     "not_implemented",
			"image_path": imagePath,
			"prompt":     prompt,
		},
	}, nil
}

// =============================================================================
// VideoGenTool
// =============================================================================

// VideoGenTool generates videos (placeholder).
type VideoGenTool struct {
	logger *logrus.Logger
}

// NewVideoGenTool creates a new VideoGenTool.
func NewVideoGenTool(logger *logrus.Logger) *VideoGenTool {
	return &VideoGenTool{logger: logger}
}

func (t *VideoGenTool) Name() string {
	return "video_gen"
}

func (t *VideoGenTool) Description() string {
	return "Generates videos from text descriptions. This feature is not yet implemented and will be available in a future update."
}

func (t *VideoGenTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "A text description of the video to generate.",
			},
			"duration": map[string]interface{}{
				"type":        "integer",
				"description": "The desired video duration in seconds.",
			},
			"resolution": map[string]interface{}{
				"type":        "string",
				"description": "The desired video resolution (e.g., '1080p').",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *VideoGenTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	prompt := ""
	if p, ok := ctx.Params["prompt"].(string); ok {
		prompt = p
	}

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"prompt":     prompt,
	}).Info("video generation requested (not yet implemented)")

	return tool.ToolResult{
		Success: true,
		Output:  "Video generation is not yet implemented. This feature is planned for a future release. You can use external services like Runway, Pika, or Sora for video generation.",
		Data: map[string]interface{}{
			"status": "not_implemented",
			"prompt": prompt,
		},
	}, nil
}

// =============================================================================
// MermaidTool
// =============================================================================

// MermaidTool renders mermaid diagrams (placeholder).
type MermaidTool struct {
	logger *logrus.Logger
}

// NewMermaidTool creates a new MermaidTool.
func NewMermaidTool(logger *logrus.Logger) *MermaidTool {
	return &MermaidTool{logger: logger}
}

func (t *MermaidTool) Name() string {
	return "mermaid"
}

func (t *MermaidTool) Description() string {
	return "Renders Mermaid.js diagrams and charts. This feature is not yet implemented and will be available in a future update."
}

func (t *MermaidTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"code": map[string]interface{}{
				"type":        "string",
				"description": "The Mermaid.js diagram code to render.",
			},
			"output_format": map[string]interface{}{
				"type":        "string",
				"description": "The desired output format (e.g., 'svg', 'png').",
				"enum":        []string{"svg", "png"},
			},
		},
		"required": []string{"code"},
	}
}

func (t *MermaidTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	code := ""
	if c, ok := ctx.Params["code"].(string); ok {
		code = c
	}

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"code_len":   len(code),
	}).Info("mermaid diagram rendering requested (not yet implemented)")

	return tool.ToolResult{
		Success: true,
		Output:  "Mermaid diagram rendering is not yet implemented. This feature is planned for a future release. You can use the Mermaid Live Editor (https://mermaid.live) to render diagrams online.",
		Data: map[string]interface{}{
			"status": "not_implemented",
			"code":   code,
		},
	}, nil
}

// =============================================================================
// MemoryTool
// =============================================================================

// MemoryTool stores and retrieves memories per session.
type MemoryTool struct {
	logger   *logrus.Logger
	memories map[string]map[string]string // sessionID -> {key: value}
	mu       sync.RWMutex
}

// NewMemoryTool creates a new MemoryTool.
func NewMemoryTool(logger *logrus.Logger) *MemoryTool {
	return &MemoryTool{
		logger:   logger,
		memories: make(map[string]map[string]string),
	}
}

func (t *MemoryTool) Name() string {
	return "memory"
}

func (t *MemoryTool) Description() string {
	return "Stores, retrieves, and searches key-value memories for the current session. Use this to remember important information across messages. Actions: 'store' (save a memory), 'get' (retrieve by key), 'search' (find keys containing a substring)."
}

func (t *MemoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The action to perform: 'store', 'get', or 'search'.",
				"enum":        []string{"store", "get", "search"},
			},
			"key": map[string]interface{}{
				"type":        "string",
				"description": "The key for the memory (required for 'store' and 'get').",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "The value to store (required for 'store').",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query substring (required for 'search').",
			},
		},
		"required": []string{"action"},
	}
}

func (t *MemoryTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	action, ok := ctx.Params["action"].(string)
	if !ok || action == "" {
		return tool.ToolResult{
			Success: false,
			Output:  "No action specified.",
			Error:   "Missing required parameter: action",
		}, nil
	}

	switch action {
	case "store":
		return t.handleStore(ctx)
	case "get":
		return t.handleGet(ctx)
	case "search":
		return t.handleSearch(ctx)
	default:
		return tool.ToolResult{
			Success: false,
			Output:  fmt.Sprintf("Unknown action: %s", action),
			Error:   fmt.Sprintf("Unknown action '%s'. Supported actions: store, get, search.", action),
		}, nil
	}
}

func (t *MemoryTool) handleStore(ctx tool.ToolContext) (tool.ToolResult, error) {
	key, ok := ctx.Params["key"].(string)
	if !ok || key == "" {
		return tool.ToolResult{
			Success: false,
			Output:  "No key provided.",
			Error:   "Missing required parameter: key",
		}, nil
	}

	value, ok := ctx.Params["value"].(string)
	if !ok {
		return tool.ToolResult{
			Success: false,
			Output:  "No value provided.",
			Error:   "Missing required parameter: value",
		}, nil
	}

	t.mu.Lock()
	if t.memories[ctx.SessionID] == nil {
		t.memories[ctx.SessionID] = make(map[string]string)
	}
	t.memories[ctx.SessionID][key] = value
	t.mu.Unlock()

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"action":     "store",
		"key":        key,
	}).Info("memory stored")

	return tool.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Memory stored: %s = %s", key, value),
		Data: map[string]interface{}{
			"action": "store",
			"key":    key,
			"value":  value,
		},
	}, nil
}

func (t *MemoryTool) handleGet(ctx tool.ToolContext) (tool.ToolResult, error) {
	key, ok := ctx.Params["key"].(string)
	if !ok || key == "" {
		return tool.ToolResult{
			Success: false,
			Output:  "No key provided.",
			Error:   "Missing required parameter: key",
		}, nil
	}

	t.mu.RLock()
	value, found := t.memories[ctx.SessionID][key]
	t.mu.RUnlock()

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"action":     "get",
		"key":        key,
		"found":      found,
	}).Info("memory retrieval")

	if !found {
		return tool.ToolResult{
			Success: true,
			Output:  fmt.Sprintf("No memory found for key: %s", key),
			Data: map[string]interface{}{
				"action": "get",
				"key":    key,
				"found":  false,
			},
		}, nil
	}

	return tool.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Memory: %s = %s", key, value),
		Data: map[string]interface{}{
			"action": "get",
			"key":    key,
			"value":  value,
			"found":  true,
		},
	}, nil
}

func (t *MemoryTool) handleSearch(ctx tool.ToolContext) (tool.ToolResult, error) {
	query, ok := ctx.Params["query"].(string)
	if !ok || query == "" {
		return tool.ToolResult{
			Success: false,
			Output:  "No query provided.",
			Error:   "Missing required parameter: query",
		}, nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	sessionMemories := t.memories[ctx.SessionID]
	results := make(map[string]string)
	queryLower := strings.ToLower(query)

	for key, value := range sessionMemories {
		if strings.Contains(strings.ToLower(key), queryLower) || strings.Contains(strings.ToLower(value), queryLower) {
			results[key] = value
		}
	}

	t.logger.WithFields(logrus.Fields{
		"session_id":  ctx.SessionID,
		"action":      "search",
		"query":       query,
		"match_count": len(results),
	}).Info("memory search")

	if len(results) == 0 {
		return tool.ToolResult{
			Success: true,
			Output:  fmt.Sprintf("No memories found matching query: %s", query),
			Data: map[string]interface{}{
				"action": "search",
				"query":  query,
				"results": map[string]string{},
			},
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories matching '%s':\n", len(results), query))
	for k, v := range results {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
	}

	return tool.ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"action":  "search",
			"query":   query,
			"results": results,
		},
	}, nil
}

// =============================================================================
// LSPTool
// =============================================================================

// LSPTool provides Language Server Protocol integration (placeholder).
type LSPTool struct {
	logger *logrus.Logger
}

// NewLSPTool creates a new LSPTool.
func NewLSPTool(logger *logrus.Logger) *LSPTool {
	return &LSPTool{logger: logger}
}

func (t *LSPTool) Name() string {
	return "lsp"
}

func (t *LSPTool) Description() string {
	return "Provides Language Server Protocol integration for code intelligence features like go-to-definition, find references, hover information, and diagnostics. This feature is not yet implemented and will be available in a future update."
}

func (t *LSPTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The LSP action to perform: 'definition', 'references', 'hover', 'diagnostics', 'completion'.",
				"enum":        []string{"definition", "references", "hover", "diagnostics", "completion"},
			},
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the source file.",
			},
			"line": map[string]interface{}{
				"type":        "integer",
				"description": "The line number (1-based) in the file.",
			},
			"character": map[string]interface{}{
				"type":        "integer",
				"description": "The character offset (1-based) on the line.",
			},
		},
		"required": []string{"action", "file_path", "line", "character"},
	}
}

func (t *LSPTool) Execute(ctx tool.ToolContext) (tool.ToolResult, error) {
	action := ""
	if a, ok := ctx.Params["action"].(string); ok {
		action = a
	}
	filePath := ""
	if p, ok := ctx.Params["file_path"].(string); ok {
		filePath = p
	}

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"action":     action,
		"file_path":  filePath,
	}).Info("LSP action requested (not yet implemented)")

	return tool.ToolResult{
		Success: true,
		Output:  "LSP integration is not yet implemented. This feature is planned for a future release. LSP support will provide code intelligence features like go-to-definition, find references, hover information, and diagnostics.",
		Data: map[string]interface{}{
			"status":    "not_implemented",
			"action":    action,
			"file_path": filePath,
		},
	}, nil
}