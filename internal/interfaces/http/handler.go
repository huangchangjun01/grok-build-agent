package http

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/spacexc/grok-build/internal/application/agent"
	"github.com/spacexc/grok-build/internal/application/session"
	apptool "github.com/spacexc/grok-build/internal/application/tool"
	domainsession "github.com/spacexc/grok-build/internal/domain/session"
	domaintool "github.com/spacexc/grok-build/internal/domain/tool"
	"github.com/spacexc/grok-build/internal/infrastructure/fs"
	"github.com/spacexc/grok-build/internal/infrastructure/git"
	"github.com/spacexc/grok-build/internal/infrastructure/llm"
	"github.com/spacexc/grok-build/internal/infrastructure/mcp"
	"github.com/spacexc/grok-build/internal/infrastructure/persistence"
	"github.com/spacexc/grok-build/internal/infrastructure/shell"
	"github.com/spacexc/grok-build/internal/infrastructure/telemetry"
	"github.com/spacexc/grok-build/internal/infrastructure/web"
	"github.com/spacexc/grok-build/pkg/config"
)

// Handler holds all HTTP handlers and dependencies.
type Handler struct {
	config         *config.Config
	db             *sql.DB
	logger         *logrus.Logger
	llmClient      *llm.Client
	agent          *agent.Agent
	toolRegistry   *apptool.ToolRegistry
	sessionSvc     *session.Service
	sessionRepo    *persistence.SessionRepo
	fileReader     *fs.FileReader
	fileWriter     *fs.FileWriter
	dirLister      *fs.DirLister
	grepSearcher   *fs.GrepSearcher
	globSearcher   *fs.GlobSearcher
	searchReplacer *fs.SearchReplacer
	shellExecutor  *shell.Executor
	webSearchClient *web.SearchClient
	webFetchClient *web.FetchClient
	gitClient      *git.Client
	mcpClient      *mcp.MCPClient
	telemetry      *telemetry.Telemetry
	goalManager     *agent.GoalManager
	planModeManager *agent.PlanModeManager
	subAgentManager *agent.SubAgentManager
	pluginManager   *agent.PluginManager
	skillManager    *agent.SkillManager
	lazinessDetector *agent.LazinessDetector
	upgrader        websocket.Upgrader
}

// NewHandler creates and wires all dependencies.
func NewHandler(cfg *config.Config, db *sql.DB, logger *logrus.Logger) *Handler {
	// 1. Create infrastructure
	llmClient := llm.NewClient(&cfg.LLM, logger)
	sessionRepo := persistence.NewSessionRepo(db, cfg.Storage.JSONLDir, cfg.Storage.SessionsDir, logger)
	fileReader := fs.NewFileReader(logger)
	fileWriter := fs.NewFileWriter(logger)
	dirLister := fs.NewDirLister(logger)
	grepSearcher := fs.NewGrepSearcher(logger)
	globSearcher := fs.NewGlobSearcher(logger)
	searchReplacer := fs.NewSearchReplacer(logger)
	shellExecutor := shell.NewExecutor(logger)
	webSearchClient := web.NewSearchClient(logger)
	webFetchClient := web.NewFetchClient(logger)
	gitClient := git.NewClient(logger)
	mcpClient := mcp.NewMCPClient(logger)
	tel := telemetry.NewTelemetry(logger)

	// 2. Create application services
	toolRegistry := apptool.NewToolRegistry(logger)
	sessionSvc := session.NewService(sessionRepo, llmClient, cfg, logger)
	goalManager := agent.NewGoalManager(logger)
	planModeManager := agent.NewPlanModeManager(logger)
	pluginManager := agent.NewPluginManager(logger)
	skillManager := agent.NewSkillManager(logger)
	lazinessDetector := agent.NewLazinessDetector(logger)

	// 3. Create agent
	ag := agent.NewAgent(cfg, llmClient, toolRegistry, sessionRepo, logger)
	subAgentManager := agent.NewSubAgentManager(ag, sessionSvc, cfg, logger)

	// 4. Register all tools
	registerAllTools(
		toolRegistry, fileReader, fileWriter, dirLister, grepSearcher,
		globSearcher, searchReplacer, shellExecutor, webSearchClient,
		webFetchClient, gitClient, goalManager, planModeManager, logger,
	)

	return &Handler{
		config:         cfg,
		db:             db,
		logger:         logger,
		llmClient:      llmClient,
		agent:          ag,
		toolRegistry:   toolRegistry,
		sessionSvc:     sessionSvc,
		sessionRepo:    sessionRepo,
		fileReader:     fileReader,
		fileWriter:     fileWriter,
		dirLister:      dirLister,
		grepSearcher:   grepSearcher,
		globSearcher:   globSearcher,
		searchReplacer: searchReplacer,
		shellExecutor:  shellExecutor,
		webSearchClient: webSearchClient,
		webFetchClient: webFetchClient,
		gitClient:      gitClient,
		mcpClient:      mcpClient,
		telemetry:      tel,
		goalManager:     goalManager,
		planModeManager: planModeManager,
		subAgentManager: subAgentManager,
		pluginManager:   pluginManager,
		skillManager:    skillManager,
		lazinessDetector: lazinessDetector,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// registerAllTools registers all 24 tools into the registry.
func registerAllTools(
	registry *apptool.ToolRegistry,
	fileReader *fs.FileReader,
	fileWriter *fs.FileWriter,
	dirLister *fs.DirLister,
	grepSearcher *fs.GrepSearcher,
	globSearcher *fs.GlobSearcher,
	searchReplacer *fs.SearchReplacer,
	shellExecutor *shell.Executor,
	webSearchClient *web.SearchClient,
	webFetchClient *web.FetchClient,
	gitClient *git.Client,
	goalManager *agent.GoalManager,
	planModeManager *agent.PlanModeManager,
	logger *logrus.Logger,
) {
	// Core tools (from tools.go)
	registry.Register(apptool.NewReadFileTool(fileReader, logger))
	registry.Register(apptool.NewWriteFileTool(fileWriter, logger))
	registry.Register(apptool.NewSearchReplaceTool(searchReplacer, logger))
	registry.Register(apptool.NewListDirTool(dirLister, logger))
	registry.Register(apptool.NewGrepTool(grepSearcher, logger))
	registry.Register(apptool.NewGlobTool(globSearcher, logger))
	registry.Register(apptool.NewBashTool(shellExecutor, logger))
	registry.Register(apptool.NewWebSearchTool(webSearchClient, logger))
	registry.Register(apptool.NewWebFetchTool(webFetchClient, logger))
	registry.Register(apptool.NewTodoWriteTool(logger))
	registry.Register(apptool.NewTaskTool(shellExecutor, logger))
	registry.Register(apptool.NewAskUserQuestionTool(logger))
	registry.Register(apptool.NewKillTaskTool(shellExecutor, logger))
	registry.Register(apptool.NewTaskOutputTool(shellExecutor, logger))

	// Advanced tools (from advanced.go)
	registry.Register(apptool.NewEnterPlanModeTool(logger, func(sessionID string, enabled bool) error {
		if enabled {
			return planModeManager.EnterPlanMode(sessionID)
		}
		return planModeManager.ExitPlanMode(sessionID)
	}))
	registry.Register(apptool.NewExitPlanModeTool(logger, func(sessionID string, enabled bool) error {
		if enabled {
			return planModeManager.EnterPlanMode(sessionID)
		}
		return planModeManager.ExitPlanMode(sessionID)
	}))
	registry.Register(apptool.NewUpdateGoalTool(logger, func(sessionID string, goal string) error {
		goalManager.CreateGoal(sessionID, goal)
		return nil
	}))
	registry.Register(apptool.NewSchedulerTool(shellExecutor, logger))
	registry.Register(apptool.NewImageGenTool(logger))
	registry.Register(apptool.NewImageEditTool(logger))
	registry.Register(apptool.NewVideoGenTool(logger))
	registry.Register(apptool.NewMermaidTool(logger))
	registry.Register(apptool.NewMemoryTool(logger))
	registry.Register(apptool.NewLSPTool(logger))

	// Git tool
	registry.Register(newGitTool(gitClient, logger))
}

// RegisterRoutes registers all API routes on the gin router.
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api")
	{
		// Sessions
		api.POST("/sessions", h.CreateSession)
		api.GET("/sessions", h.ListSessions)
		api.GET("/sessions/search", h.SearchSessions)
		api.GET("/sessions/:id", h.GetSession)
		api.DELETE("/sessions/:id", h.DeleteSession)
		api.PUT("/sessions/:id", h.RenameSession)
		api.POST("/sessions/:id/fork", h.ForkSession)
		api.POST("/sessions/:id/rewind", h.RewindSession)
		api.GET("/sessions/:id/export", h.ExportSession)
		api.GET("/sessions/:id/messages", h.GetMessages)
		api.POST("/sessions/:id/summary", h.GenerateSummary)

		// Chat
		api.POST("/sessions/:id/chat", h.Chat)

		// Tools
		api.GET("/tools", h.ListTools)
		api.POST("/tools/:name", h.ExecuteTool)

		// Workspace
		api.GET("/workspace/status", h.WorkspaceStatus)
		api.GET("/workspace/files", h.WorkspaceFiles)
		api.GET("/workspace/git", h.WorkspaceGit)

		// Config
		api.GET("/config", h.GetConfig)
		api.PUT("/config", h.UpdateConfig)

		// Goals
		api.GET("/sessions/:id/goals", h.GetGoals)
		api.POST("/sessions/:id/goals", h.CreateGoal)

		// Plan Mode
		api.GET("/sessions/:id/plan-mode", h.GetPlanMode)
		api.POST("/sessions/:id/plan-mode/enter", h.EnterPlanMode)
		api.POST("/sessions/:id/plan-mode/exit", h.ExitPlanMode)

		// SubAgents
		api.GET("/sessions/:id/subagents", h.ListSubAgents)

		// MCP
		api.GET("/mcp/servers", h.ListMCPServers)
		api.POST("/mcp/servers", h.StartMCPServer)
		api.DELETE("/mcp/servers/:name", h.StopMCPServer)

		// Plugins & Skills
		api.GET("/plugins", h.ListPlugins)
		api.GET("/skills", h.ListSkills)
		api.GET("/skills/search", h.SearchSkills)

		// Telemetry
		api.GET("/telemetry", h.GetTelemetry)
	}

	// WebSocket
	router.GET("/ws/sessions/:id/chat", h.WebSocketChat)
}

// =============================================================================
// Session Handlers
// =============================================================================

// CreateSession handles POST /api/sessions.
func (h *Handler) CreateSession(c *gin.Context) {
	var req struct {
		WorkDir string `json:"work_dir"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.WorkDir = h.config.Workspace.DefaultWorkDir
	}
	if req.WorkDir == "" {
		req.WorkDir = "."
	}

	sess, err := h.sessionSvc.CreateSession(req.WorkDir)
	if err != nil {
		h.logger.WithError(err).Error("CreateSession failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.telemetry.TrackSession(sess.ID)
	c.JSON(http.StatusCreated, sess)
}

// ListSessions handles GET /api/sessions.
func (h *Handler) ListSessions(c *gin.Context) {
	filter := &domainsession.SessionFilter{
		Limit:   50,
		OrderBy: "updated_at",
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}
	if orderBy := c.Query("order_by"); orderBy != "" {
		filter.OrderBy = orderBy
	}

	sessions, err := h.sessionSvc.ListSessions(filter)
	if err != nil {
		h.logger.WithError(err).Error("ListSessions failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if sessions == nil {
		sessions = []*domainsession.Session{}
	}

	c.JSON(http.StatusOK, sessions)
}

// SearchSessions handles GET /api/sessions/search.
func (h *Handler) SearchSessions(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	sessions, err := h.sessionSvc.SearchSessions(query)
	if err != nil {
		h.logger.WithError(err).Error("SearchSessions failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if sessions == nil {
		sessions = []*domainsession.Session{}
	}

	c.JSON(http.StatusOK, sessions)
}

// GetSession handles GET /api/sessions/:id.
func (h *Handler) GetSession(c *gin.Context) {
	id := c.Param("id")
	sess, err := h.sessionSvc.GetSession(id)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", id).Error("GetSession failed")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, sess)
}

// DeleteSession handles DELETE /api/sessions/:id.
func (h *Handler) DeleteSession(c *gin.Context) {
	id := c.Param("id")
	if err := h.sessionSvc.DeleteSession(id); err != nil {
		h.logger.WithError(err).WithField("session_id", id).Error("DeleteSession failed")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	h.telemetry.UntrackSession(id)
	c.JSON(http.StatusOK, gin.H{"message": "session deleted"})
}

// RenameSession handles PUT /api/sessions/:id.
func (h *Handler) RenameSession(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	if err := h.sessionSvc.RenameSession(id, req.Title); err != nil {
		h.logger.WithError(err).WithField("session_id", id).Error("RenameSession failed")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "session renamed"})
}

// ForkSession handles POST /api/sessions/:id/fork.
func (h *Handler) ForkSession(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		FromMessageIndex int `json:"from_message_index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.FromMessageIndex = 0
	}

	newSession, err := h.sessionSvc.ForkSession(id, req.FromMessageIndex)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", id).Error("ForkSession failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.telemetry.TrackSession(newSession.ID)
	c.JSON(http.StatusCreated, newSession)
}

// RewindSession handles POST /api/sessions/:id/rewind.
func (h *Handler) RewindSession(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		ToMessageIndex int `json:"to_message_index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to_message_index is required"})
		return
	}

	if err := h.sessionSvc.RewindSession(id, req.ToMessageIndex); err != nil {
		h.logger.WithError(err).WithField("session_id", id).Error("RewindSession failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "session rewound"})
}

// ExportSession handles GET /api/sessions/:id/export.
func (h *Handler) ExportSession(c *gin.Context) {
	id := c.Param("id")
	format := c.Query("format")
	if format == "" {
		format = "json"
	}

	data, contentType, err := h.sessionSvc.ExportSession(id, format)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", id).Error("ExportSession failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="session-%s.%s"`, id, format))
	c.Data(http.StatusOK, contentType, data)
}

// GetMessages handles GET /api/sessions/:id/messages.
func (h *Handler) GetMessages(c *gin.Context) {
	id := c.Param("id")
	limit := 0
	offset := 0

	if limitStr := c.Query("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			offset = v
		}
	}

	messages, err := h.sessionSvc.GetMessages(id, limit, offset)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", id).Error("GetMessages failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if messages == nil {
		messages = []*domainsession.Message{}
	}

	c.JSON(http.StatusOK, messages)
}

// GenerateSummary handles POST /api/sessions/:id/summary.
func (h *Handler) GenerateSummary(c *gin.Context) {
	id := c.Param("id")
	title, err := h.sessionSvc.GenerateSummary(id)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", id).Error("GenerateSummary failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"title": title})
}

// =============================================================================
// Chat Handlers
// =============================================================================

// Chat handles POST /api/sessions/:id/chat (non-streaming).
func (h *Handler) Chat(c *gin.Context) {
	sessionID := c.Param("id")
	startTime := time.Now()

	var req struct {
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}

	ctx := c.Request.Context()
	result, err := h.agent.Run(ctx, sessionID, req.Message, nil)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", sessionID).Error("Chat failed")
		h.telemetry.RecordError(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.telemetry.RecordRequest(time.Since(startTime))

	c.JSON(http.StatusOK, gin.H{
		"role":    result.Role,
		"content": result.Content,
	})
}

// WebSocketChat handles WS /ws/sessions/:id/chat.
func (h *Handler) WebSocketChat(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.WithError(err).Error("WebSocket upgrade failed")
		return
	}
	defer conn.Close()

	sessionID := c.Param("id")
	streamCh := make(chan string, 100)
	ctx := c.Request.Context()

	// Read messages from WebSocket
	go func() {
		defer func() {
			// Don't close streamCh here - it's shared across messages
		}()
		for {
			_, msgBytes, err := conn.ReadMessage()
			if err != nil {
				h.logger.WithError(err).WithField("session_id", sessionID).Debug("WebSocket read closed")
				return
			}

			var req struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(msgBytes, &req); err != nil {
				h.logger.WithError(err).Debug("failed to parse WebSocket message")
				conn.WriteJSON(map[string]string{"type": "error", "content": "invalid message format"})
				continue
			}

			if req.Message == "" {
				conn.WriteJSON(map[string]string{"type": "error", "content": "message is required"})
				continue
			}

			// Run agent in goroutine
			go func(msg string) {
				startTime := time.Now()
				_, err := h.agent.Run(ctx, sessionID, msg, streamCh)
				if err != nil {
					h.logger.WithError(err).WithField("session_id", sessionID).Error("WebSocket agent run failed")
					h.telemetry.RecordError(err)
					select {
					case streamCh <- fmt.Sprintf("Error: %v", err):
					default:
					}
				}
				h.telemetry.RecordRequest(time.Since(startTime))
			}(req.Message)
		}
	}()

	// Send streaming chunks to WebSocket
	for chunk := range streamCh {
		if err := conn.WriteJSON(map[string]string{"type": "chunk", "content": chunk}); err != nil {
			h.logger.WithError(err).WithField("session_id", sessionID).Debug("WebSocket write failed")
			return
		}
	}
}

// =============================================================================
// Tool Handlers
// =============================================================================

// ListTools handles GET /api/tools.
func (h *Handler) ListTools(c *gin.Context) {
	tools := h.toolRegistry.List()
	type toolInfo struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	}
	result := make([]toolInfo, 0, len(tools))
	for _, t := range tools {
		result = append(result, toolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	c.JSON(http.StatusOK, result)
}

// ExecuteTool handles POST /api/tools/:name.
func (h *Handler) ExecuteTool(c *gin.Context) {
	toolName := c.Param("name")

	t, ok := h.toolRegistry.Get(toolName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("tool not found: %s", toolName)})
		return
	}

	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		params = make(map[string]interface{})
	}

	ctx := domaintool.ToolContext{
		SessionID: c.Query("session_id"),
		WorkDir:   c.Query("work_dir"),
		Params:    params,
	}
	if ctx.WorkDir == "" {
		ctx.WorkDir = h.config.Workspace.DefaultWorkDir
	}

	result, err := t.Execute(ctx)
	if err != nil {
		h.logger.WithError(err).WithField("tool", toolName).Error("ExecuteTool failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.telemetry.RecordToolCall()
	c.JSON(http.StatusOK, result)
}

// =============================================================================
// Workspace Handlers
// =============================================================================

// WorkspaceStatus handles GET /api/workspace/status.
func (h *Handler) WorkspaceStatus(c *gin.Context) {
	workDir := c.Query("work_dir")
	if workDir == "" {
		workDir = h.config.Workspace.DefaultWorkDir
	}

	files, err := h.dirLister.ListDir(workDir)
	if err != nil {
		h.logger.WithError(err).WithField("work_dir", workDir).Error("WorkspaceStatus failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"work_dir": workDir,
		"files":    files,
	})
}

// WorkspaceFiles handles GET /api/workspace/files.
func (h *Handler) WorkspaceFiles(c *gin.Context) {
	workDir := c.Query("work_dir")
	if workDir == "" {
		workDir = h.config.Workspace.DefaultWorkDir
	}
	pattern := c.Query("pattern")
	if pattern == "" {
		pattern = "*"
	}

	matches, err := h.globSearcher.Search(pattern, workDir)
	if err != nil {
		h.logger.WithError(err).WithFields(logrus.Fields{
			"work_dir": workDir,
			"pattern":  pattern,
		}).Error("WorkspaceFiles failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if matches == nil {
		matches = []string{}
	}

	c.JSON(http.StatusOK, gin.H{
		"work_dir": workDir,
		"pattern":  pattern,
		"files":    matches,
	})
}

// WorkspaceGit handles GET /api/workspace/git.
func (h *Handler) WorkspaceGit(c *gin.Context) {
	workDir := c.Query("work_dir")
	if workDir == "" {
		workDir = h.config.Workspace.DefaultWorkDir
	}

	if !h.gitClient.IsRepo(workDir) {
		c.JSON(http.StatusOK, gin.H{
			"is_repo": false,
			"work_dir": workDir,
		})
		return
	}

	status, err := h.gitClient.GetStatus(workDir)
	if err != nil {
		h.logger.WithError(err).WithField("work_dir", workDir).Error("WorkspaceGit failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"is_repo": true,
		"work_dir": workDir,
		"status":   status,
	})
}

// =============================================================================
// Config Handlers
// =============================================================================

// GetConfig handles GET /api/config.
func (h *Handler) GetConfig(c *gin.Context) {
	// Return a sanitized version of config (no secrets)
	c.JSON(http.StatusOK, gin.H{
		"server":    h.config.Server,
		"workspace": h.config.Workspace,
		"agent":     h.config.Agent,
		"tools":     h.config.Tools,
		"llm": gin.H{
			"provider":     h.config.LLM.Provider,
			"default_model": h.config.LLM.DefaultModel,
			"models":       h.config.LLM.Models,
		},
	})
}

// UpdateConfig handles PUT /api/config.
func (h *Handler) UpdateConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "config update not yet implemented"})
}

// =============================================================================
// Goal Handlers
// =============================================================================

// GetGoals handles GET /api/sessions/:id/goals.
func (h *Handler) GetGoals(c *gin.Context) {
	sessionID := c.Param("id")
	goals := h.goalManager.GetGoals(sessionID)
	if goals == nil {
		goals = []*agent.Goal{}
	}

	summary := h.goalManager.SummarizeGoal(sessionID)

	c.JSON(http.StatusOK, gin.H{
		"goals":   goals,
		"summary": summary,
	})
}

// CreateGoal handles POST /api/sessions/:id/goals.
func (h *Handler) CreateGoal(c *gin.Context) {
	sessionID := c.Param("id")
	var req struct {
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
		return
	}

	goal := h.goalManager.CreateGoal(sessionID, req.Description)
	c.JSON(http.StatusCreated, goal)
}

// =============================================================================
// Plan Mode Handlers
// =============================================================================

// GetPlanMode handles GET /api/sessions/:id/plan-mode.
func (h *Handler) GetPlanMode(c *gin.Context) {
	sessionID := c.Param("id")
	enabled := h.planModeManager.IsInPlanMode(sessionID)
	approved := h.planModeManager.IsApproved(sessionID)
	plan := h.planModeManager.GetPlan(sessionID)

	c.JSON(http.StatusOK, gin.H{
		"enabled":  enabled,
		"approved": approved,
		"plan":     plan,
	})
}

// EnterPlanMode handles POST /api/sessions/:id/plan-mode/enter.
func (h *Handler) EnterPlanMode(c *gin.Context) {
	sessionID := c.Param("id")
	if err := h.planModeManager.EnterPlanMode(sessionID); err != nil {
		h.logger.WithError(err).WithField("session_id", sessionID).Error("EnterPlanMode failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "entered plan mode"})
}

// ExitPlanMode handles POST /api/sessions/:id/plan-mode/exit.
func (h *Handler) ExitPlanMode(c *gin.Context) {
	sessionID := c.Param("id")
	if err := h.planModeManager.ExitPlanMode(sessionID); err != nil {
		h.logger.WithError(err).WithField("session_id", sessionID).Error("ExitPlanMode failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "exited plan mode"})
}

// =============================================================================
// SubAgent Handlers
// =============================================================================

// ListSubAgents handles GET /api/sessions/:id/subagents.
func (h *Handler) ListSubAgents(c *gin.Context) {
	sessionID := c.Param("id")
	subAgents := h.subAgentManager.ListSubAgents(sessionID)
	if subAgents == nil {
		subAgents = []*agent.SubAgentContext{}
	}

	c.JSON(http.StatusOK, subAgents)
}

// =============================================================================
// MCP Handlers
// =============================================================================

// ListMCPServers handles GET /api/mcp/servers.
func (h *Handler) ListMCPServers(c *gin.Context) {
	servers := h.mcpClient.ListServers()
	if servers == nil {
		servers = []*mcp.MCPServer{}
	}

	c.JSON(http.StatusOK, servers)
}

// StartMCPServer handles POST /api/mcp/servers.
func (h *Handler) StartMCPServer(c *gin.Context) {
	var req struct {
		Name    string            `json:"name"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Name == "" || req.Command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and command are required"})
		return
	}
	if req.Args == nil {
		req.Args = []string{}
	}

	ctx := c.Request.Context()
	if err := h.mcpClient.StartServer(ctx, req.Name, req.Command, req.Args, req.Env); err != nil {
		h.logger.WithError(err).WithField("server_name", req.Name).Error("StartMCPServer failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": fmt.Sprintf("MCP server '%s' started", req.Name)})
}

// StopMCPServer handles DELETE /api/mcp/servers/:name.
func (h *Handler) StopMCPServer(c *gin.Context) {
	name := c.Param("name")
	if err := h.mcpClient.StopServer(name); err != nil {
		h.logger.WithError(err).WithField("server_name", name).Error("StopMCPServer failed")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("MCP server '%s' stopped", name)})
}

// =============================================================================
// Plugin & Skill Handlers
// =============================================================================

// ListPlugins handles GET /api/plugins.
func (h *Handler) ListPlugins(c *gin.Context) {
	plugins := h.pluginManager.ListPlugins()
	if plugins == nil {
		plugins = []*agent.Plugin{}
	}

	c.JSON(http.StatusOK, plugins)
}

// ListSkills handles GET /api/skills.
func (h *Handler) ListSkills(c *gin.Context) {
	skills := h.skillManager.ListSkills()
	if skills == nil {
		skills = []*agent.Skill{}
	}

	c.JSON(http.StatusOK, skills)
}

// SearchSkills handles GET /api/skills/search.
func (h *Handler) SearchSkills(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	skills := h.skillManager.SearchSkills(query)
	if skills == nil {
		skills = []*agent.Skill{}
	}

	c.JSON(http.StatusOK, skills)
}

// =============================================================================
// Telemetry Handler
// =============================================================================

// GetTelemetry handles GET /api/telemetry.
func (h *Handler) GetTelemetry(c *gin.Context) {
	metrics := h.telemetry.GetMetrics()
	c.JSON(http.StatusOK, metrics)
}

// =============================================================================
// GitTool - wraps git.Client as a domain tool
// =============================================================================

// gitTool wraps git.Client to implement the Tool interface.
type gitTool struct {
	client *git.Client
	logger *logrus.Logger
}

func newGitTool(client *git.Client, logger *logrus.Logger) *gitTool {
	return &gitTool{client: client, logger: logger}
}

func (t *gitTool) Name() string { return "git" }

func (t *gitTool) Description() string {
	return "Provides git repository operations: status, diff, and tracked files listing. Use this to check the state of a git repository."
}

func (t *gitTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The git action to perform: 'status', 'diff', or 'tracked_files'.",
				"enum":        []string{"status", "diff", "tracked_files"},
			},
			"repo_path": map[string]interface{}{
				"type":        "string",
				"description": "The path to the git repository. Defaults to the current working directory.",
			},
			"staged": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, show staged diff instead of working tree diff (only for 'diff' action).",
			},
		},
		"required": []string{"action"},
	}
}

func (t *gitTool) Execute(ctx domaintool.ToolContext) (domaintool.ToolResult, error) {
	action, _ := ctx.Params["action"].(string)
	repoPath, _ := ctx.Params["repo_path"].(string)
	if repoPath == "" {
		repoPath = ctx.WorkDir
	}

	switch action {
	case "status":
		status, err := t.client.GetStatus(repoPath)
		if err != nil {
			return domaintool.ToolResult{Success: false, Error: err.Error()}, nil
		}
		output := fmt.Sprintf("Branch: %s\nClean: %v\nModified: %v\nAdded: %v\nDeleted: %v\nUntracked: %v",
			status.Branch, status.IsClean, status.Modified, status.Added, status.Deleted, status.Untracked)
		return domaintool.ToolResult{
			Success: true,
			Output:  output,
			Data: map[string]interface{}{
				"branch":    status.Branch,
				"is_clean":  status.IsClean,
				"modified":  status.Modified,
				"added":     status.Added,
				"deleted":   status.Deleted,
				"untracked": status.Untracked,
			},
		}, nil

	case "diff":
		staged := false
		if s, ok := ctx.Params["staged"].(bool); ok {
			staged = s
		}
		diff, err := t.client.GetDiff(repoPath, staged)
		if err != nil {
			return domaintool.ToolResult{Success: false, Error: err.Error()}, nil
		}
		return domaintool.ToolResult{
			Success: true,
			Output:  diff.Diff,
			Data: map[string]interface{}{
				"staged": diff.Staged,
				"diff":   diff.Diff,
			},
		}, nil

	case "tracked_files":
		files, err := t.client.GetTrackedFiles(repoPath)
		if err != nil {
			return domaintool.ToolResult{Success: false, Error: err.Error()}, nil
		}
		output := fmt.Sprintf("Tracked files (%d):\n", len(files))
		for _, f := range files {
			output += f + "\n"
		}
		return domaintool.ToolResult{
			Success: true,
			Output:  output,
			Data: map[string]interface{}{
				"files": files,
				"count": len(files),
			},
		}, nil

	default:
		return domaintool.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("unknown git action: %s (supported: status, diff, tracked_files)", action),
		}, nil
	}
}

// Ensure Handler compiles
var _ = (*Handler)(nil)