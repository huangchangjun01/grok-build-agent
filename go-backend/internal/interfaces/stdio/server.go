package stdio

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/sirupsen/logrus"

	"github.com/spacexc/go-backend/internal/application/agent"
	"github.com/spacexc/go-backend/internal/application/session"
	"github.com/spacexc/go-backend/internal/application/tool"
	"github.com/spacexc/go-backend/internal/infrastructure/fs"
	"github.com/spacexc/go-backend/internal/infrastructure/git"
	"github.com/spacexc/go-backend/internal/infrastructure/llm"
	"github.com/spacexc/go-backend/internal/infrastructure/mcp"
	"github.com/spacexc/go-backend/internal/infrastructure/persistence"
	"github.com/spacexc/go-backend/internal/infrastructure/shell"
	"github.com/spacexc/go-backend/internal/infrastructure/telemetry"
	"github.com/spacexc/go-backend/internal/infrastructure/web"
	"github.com/spacexc/go-backend/pkg/config"
)

const (
	leaderProtocolVersion uint32 = 1
	leaderBinaryVersion          = "1.0.0-go"
)

// Server handles the Leader/Stdio protocol with ACP message routing.
type Server struct {
	cfg           *config.Config
	db            *sql.DB
	logger        *logrus.Logger
	llmClient     *llm.Client
	agent         *agent.Agent
	toolRegistry  *tool.ToolRegistry
	sessionSvc    *session.Service
	sessionRepo   *persistence.SessionRepo
	shellExecutor *shell.Executor
	planModeMgr   *agent.PlanModeManager
	goalMgr       *agent.GoalManager
	telemetry     *telemetry.Telemetry
	writer        io.Writer
	eventSeq      atomic.Uint64
	// Track per-session cancel functions
	cancelFuncs map[string]context.CancelFunc
}

// NewServer creates a new Stdio protocol server.
func NewServer(cfg *config.Config, db *sql.DB, logger *logrus.Logger) *Server {
	// Infrastructure
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
	_ = mcpClient // MCP server lifecycle managed externally
	tel := telemetry.NewTelemetry(logger)

	// Application services
	toolRegistry := tool.NewToolRegistry(logger)
	sessionSvc := session.NewService(sessionRepo, llmClient, cfg, logger)
	goalMgr := agent.NewGoalManager(logger)
	planModeMgr := agent.NewPlanModeManager(logger)

	// Agent
	ag := agent.NewAgent(cfg, llmClient, toolRegistry, sessionRepo, logger)

	// Register all tools
	registerAllTools(toolRegistry, fileReader, fileWriter, dirLister, grepSearcher, globSearcher,
		searchReplacer, shellExecutor, webSearchClient, webFetchClient, gitClient, goalMgr, planModeMgr, logger)

	return &Server{
		cfg:           cfg,
		db:            db,
		logger:        logger,
		llmClient:     llmClient,
		agent:         ag,
		toolRegistry:  toolRegistry,
		sessionSvc:    sessionSvc,
		sessionRepo:   sessionRepo,
		shellExecutor: shellExecutor,
		planModeMgr:   planModeMgr,
		goalMgr:       goalMgr,
		telemetry:     tel,
		writer:        os.Stdout,
		cancelFuncs:   make(map[string]context.CancelFunc),
	}
}

// Run starts the stdio protocol loop, reading from stdin and writing to stdout.
func (s *Server) Run() error {
	s.logger.Info("Stdio server starting, waiting for client registration...")
	reader := os.Stdin

	// Wait for Register message
	var msg ClientMessage
	if err := ReadMessage(reader, &msg); err != nil {
		return fmt.Errorf("read register: %w", err)
	}
	if msg.Type != "register" {
		return fmt.Errorf("expected register message, got: %s", msg.Type)
	}

	s.logger.WithFields(logrus.Fields{
		"client_type": msg.ClientType,
		"mode":        msg.Mode,
	}).Info("Client registered")

	// Send Registered response
	protoVer := leaderProtocolVersion
	binVer := leaderBinaryVersion
	resp := ServerMessage{
		Type:                  "registered",
		ClientID:              1,
		Ready:                 true,
		LeaderProtocolVersion: &protoVer,
		LeaderBinaryVersion:   &binVer,
	}
	if err := WriteMessage(s.writer, &resp); err != nil {
		return fmt.Errorf("write registered: %w", err)
	}

	// Send LeaderReady
	readyMsg := ServerMessage{Type: "leader_ready"}
	if err := WriteMessage(s.writer, &readyMsg); err != nil {
		return fmt.Errorf("write leader_ready: %w", err)
	}

	s.logger.Info("Leader ready, processing messages...")

	// Main message loop
	for {
		var msg ClientMessage
		if err := ReadMessage(reader, &msg); err != nil {
			if err == io.EOF {
				s.logger.Info("Client disconnected")
				return nil
			}
			s.logger.WithError(err).Error("Failed to read message")
			continue
		}

		switch msg.Type {
		case "acp":
			s.handleAcp(msg.Payload)
		case "ping":
			s.handlePing()
		case "disconnect":
			s.logger.Info("Client requested disconnect")
			return nil
		default:
			s.logger.WithField("type", msg.Type).Warn("Unknown message type")
		}
	}
}

// handleAcp processes an ACP message payload.
func (s *Server) handleAcp(payload string) {
	var acp AcpMessage
	if err := json.Unmarshal([]byte(payload), &acp); err != nil {
		s.logger.WithError(err).Error("Failed to parse ACP message")
		s.sendError(-32700, "Parse error: "+err.Error())
		return
	}

	s.logger.WithField("method", acp.MethodName).Debug("ACP message received")

	switch acp.MethodName {
	case "initialize":
		s.handleInitialize()
	case "session/new":
		s.handleSessionNew(acp.Request)
	case "session/load":
		s.handleSessionLoad(acp.Request)
	case "session/prompt":
		s.handleSessionPrompt(acp.Request)
	case "session/set_mode":
		s.handleSessionSetMode(acp.Request)
	case "session/cancel":
		s.handleSessionCancel(acp.Request)
	case "session/set_model":
		s.handleSessionSetModel(acp.Request)
	default:
		s.logger.WithField("method", acp.MethodName).Warn("Unknown ACP method")
		s.sendError(-32601, "Method not found: "+acp.MethodName)
	}
}

// handleInitialize responds with server capabilities.
func (s *Server) handleInitialize() {
	resp := InitializeResponse{
		ProtocolVersion: 1,
		ServerInfo:      "SpaceXC Grok Build Go Backend v1.0.0",
		Capabilities: map[string]bool{
			"session_new":    true,
			"session_load":   true,
			"session_prompt": true,
			"session_cancel": true,
			"session_set_mode": true,
			"session_set_model": true,
			"plan_mode":      true,
			"goal_system":    true,
			"subagent":       true,
			"mcp":            true,
		},
	}
	s.sendAcp("initialize", resp)
}

// handleSessionNew creates a new session.
func (s *Server) handleSessionNew(raw json.RawMessage) {
	var req NewSessionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		s.sendError(-32602, "Invalid params: "+err.Error())
		return
	}

	cwd := req.Cwd
	if cwd == "" {
		cwd = "."
	}

	sess, err := s.sessionSvc.CreateSession(cwd)
	if err != nil {
		s.sendError(-32603, "Failed to create session: "+err.Error())
		return
	}

	resp := NewSessionResponse{
		SessionID: sess.ID,
		Cwd:       sess.WorkDir,
	}
	s.sendAcp("session/new", resp)
	s.telemetry.TrackSession(sess.ID)
}

// handleSessionLoad loads an existing session.
func (s *Server) handleSessionLoad(raw json.RawMessage) {
	var req LoadSessionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		s.sendError(-32602, "Invalid params: "+err.Error())
		return
	}

	sess, err := s.sessionSvc.GetSession(req.SessionID)
	if err != nil {
		s.sendError(-32603, "Session not found: "+err.Error())
		return
	}

	messages, err := s.sessionSvc.GetMessages(req.SessionID, 100, 0)
	if err != nil {
		s.sendError(-32603, "Failed to load messages: "+err.Error())
		return
	}

	var payloads []AcpMessagePayload
	for _, m := range messages {
		payloads = append(payloads, AcpMessagePayload{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}

	resp := LoadSessionResponse{
		SessionID: sess.ID,
		Cwd:       sess.WorkDir,
		Messages:  payloads,
	}
	s.sendAcp("session/load", resp)
}

// handleSessionPrompt processes a user prompt and runs the agent.
func (s *Server) handleSessionPrompt(raw json.RawMessage) {
	var req PromptRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		s.sendError(-32602, "Invalid params: "+err.Error())
		return
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFuncs[req.SessionID] = cancel
	defer func() {
		cancel()
		delete(s.cancelFuncs, req.SessionID)
	}()

	// Create stream channel for sending updates to client
	streamCh := make(chan string, 100)
	errCh := make(chan error, 1)

	// Run agent in goroutine
	go func() {
		_, err := s.agent.Run(ctx, req.SessionID, req.Prompt, streamCh)
		errCh <- err
	}()

	// Stream updates to client
	eventID := uint64(0)
loop:
	for {
		select {
		case chunk, ok := <-streamCh:
			if !ok {
				break loop
			}
			eventID++
			s.sendSessionUpdate(req.SessionID, eventID, "agent_message_chunk", AgentMessageChunk{
				SessionID: req.SessionID,
				Content:   chunk,
			})
		case err := <-errCh:
			if err != nil {
				s.logger.WithError(err).Error("Agent run error")
				s.sendSessionUpdate(req.SessionID, eventID+1, "error", map[string]string{
					"session_id": req.SessionID,
					"error":      err.Error(),
				})
			}
			break loop
		}
	}

	// Send turn_complete
	s.sendSessionUpdate(req.SessionID, eventID+1, "turn_complete", map[string]string{
		"session_id": req.SessionID,
		"status":     "completed",
	})

	// Send prompt response
	s.sendAcp("session/prompt", PromptResponse{
		SessionID: req.SessionID,
		Status:    "completed",
	})
}

// handleSessionSetMode sets the session mode (plan/default).
func (s *Server) handleSessionSetMode(raw json.RawMessage) {
	var req SetSessionModeRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		s.sendError(-32602, "Invalid params: "+err.Error())
		return
	}

	switch req.Mode {
	case "plan":
		s.planModeMgr.EnterPlanMode(req.SessionID)
	case "default":
		s.planModeMgr.ExitPlanMode(req.SessionID)
	}

	s.sendAcp("session/set_mode", SetSessionModeResponse{
		SessionID: req.SessionID,
		Mode:      req.Mode,
	})
}

// handleSessionCancel cancels a running agent session.
func (s *Server) handleSessionCancel(raw json.RawMessage) {
	var req CancelNotification
	if err := json.Unmarshal(raw, &req); err != nil {
		s.sendError(-32602, "Invalid params: "+err.Error())
		return
	}

	if cancel, ok := s.cancelFuncs[req.SessionID]; ok {
		cancel()
		s.logger.WithField("session_id", req.SessionID).Info("Session cancelled")
	}

	// Send empty response (ack)
	s.sendAcp("session/cancel", map[string]string{"session_id": req.SessionID})
}

// handleSessionSetModel sets the model for a session.
func (s *Server) handleSessionSetModel(raw json.RawMessage) {
	var req SetSessionModelRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		s.sendError(-32602, "Invalid params: "+err.Error())
		return
	}

	s.sendAcp("session/set_model", SetSessionModelResponse{
		SessionID: req.SessionID,
		ModelID:   req.ModelID,
	})
}

// handlePing responds to ping with pong.
func (s *Server) handlePing() {
	resp := ServerMessage{Type: "pong"}
	if err := WriteMessage(s.writer, &resp); err != nil {
		s.logger.WithError(err).Error("Failed to send pong")
	}
}

// sendAcp sends an ACP message with the given method name and request body.
func (s *Server) sendAcp(method string, request interface{}) {
	reqJSON, err := json.Marshal(request)
	if err != nil {
		s.logger.WithError(err).Error("Failed to marshal ACP request")
		return
	}

	acpMsg := AcpMessage{
		MethodName: method,
		Request:    reqJSON,
	}
	acpJSON, err := json.Marshal(acpMsg)
	if err != nil {
		s.logger.WithError(err).Error("Failed to marshal ACP message")
		return
	}

	msg := ServerMessage{
		Type:    "acp",
		Payload: string(acpJSON),
	}
	if err := WriteMessage(s.writer, &msg); err != nil {
		s.logger.WithError(err).Error("Failed to send ACP message")
	}
}

// sendSessionUpdate sends a session update notification to the client.
func (s *Server) sendSessionUpdate(sessionID string, eventID uint64, updateType string, data interface{}) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		s.logger.WithError(err).Error("Failed to marshal session update data")
		return
	}

	update := SessionUpdate{
		SessionID: sessionID,
		EventID:   eventID,
		Type:      updateType,
		Data:      dataJSON,
	}
	s.sendAcp("session/update", update)
}

// sendError sends an error response to the client.
func (s *Server) sendError(code int, message string) {
	resp := ServerMessage{
		Type:    "error",
		Code:    code,
		Message: message,
	}
	if err := WriteMessage(s.writer, &resp); err != nil {
		s.logger.WithError(err).Error("Failed to send error message")
	}
}

// registerAllTools registers all tools with the tool registry.
func registerAllTools(
	registry *tool.ToolRegistry,
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
	goalMgr *agent.GoalManager,
	planModeMgr *agent.PlanModeManager,
	logger *logrus.Logger,
) {
	// Core tools
	registry.Register(tool.NewReadFileTool(fileReader, logger))
	registry.Register(tool.NewWriteFileTool(fileWriter, logger))
	registry.Register(tool.NewSearchReplaceTool(searchReplacer, logger))
	registry.Register(tool.NewListDirTool(dirLister, logger))
	registry.Register(tool.NewGrepTool(grepSearcher, logger))
	registry.Register(tool.NewGlobTool(globSearcher, logger))
	registry.Register(tool.NewBashTool(shellExecutor, logger))
	registry.Register(tool.NewWebSearchTool(webSearchClient, logger))
	registry.Register(tool.NewWebFetchTool(webFetchClient, logger))
	registry.Register(tool.NewTodoWriteTool(logger))
	registry.Register(tool.NewTaskTool(shellExecutor, logger))
	registry.Register(tool.NewAskUserQuestionTool(logger))
	registry.Register(tool.NewKillTaskTool(shellExecutor, logger))
	registry.Register(tool.NewTaskOutputTool(shellExecutor, logger))

	// Advanced tools
	// Plan mode callbacks
	enterPlanMode := func(sessionID string, enabled bool) error {
		return planModeMgr.EnterPlanMode(sessionID)
	}
	exitPlanMode := func(sessionID string, enabled bool) error {
		return planModeMgr.ExitPlanMode(sessionID)
	}
	// Goal callback wrapper
	updateGoal := func(sessionID string, goal string) error {
		goalMgr.CreateGoal(sessionID, goal)
		return nil
	}

	registry.Register(tool.NewEnterPlanModeTool(logger, enterPlanMode))
	registry.Register(tool.NewExitPlanModeTool(logger, exitPlanMode))
	registry.Register(tool.NewUpdateGoalTool(logger, updateGoal))
	registry.Register(tool.NewImageGenTool(logger))
	registry.Register(tool.NewImageEditTool(logger))
	registry.Register(tool.NewVideoGenTool(logger))
	registry.Register(tool.NewMermaidTool(logger))
	registry.Register(tool.NewMemoryTool(logger))
	registry.Register(tool.NewLSPTool(logger))
	registry.Register(tool.NewSchedulerTool(shellExecutor, logger))
}