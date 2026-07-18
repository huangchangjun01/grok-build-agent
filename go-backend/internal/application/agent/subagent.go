package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	sessionSvc "github.com/spacexc/go-backend/internal/application/session"
	"github.com/spacexc/go-backend/pkg/config"
)

// SubAgentManager manages sub-agents.
type SubAgentManager struct {
	agent      *Agent
	sessionSvc *sessionSvc.Service
	config     *config.Config
	logger     *logrus.Logger
	subAgents  map[string]*SubAgentContext // sessionID -> context
	mu         sync.RWMutex
}

// SubAgentContext holds the context of a sub-agent.
type SubAgentContext struct {
	ParentSessionID string    `json:"parent_session_id"`
	SessionID       string    `json:"session_id"`
	Task            string    `json:"task"`
	Status          string    `json:"status"` // "running", "completed", "failed"
	CreatedAt       time.Time `json:"created_at"`
	Result          string    `json:"result,omitempty"`
	cancel          context.CancelFunc
}

// NewSubAgentManager creates a new SubAgentManager.
func NewSubAgentManager(agent *Agent, sessionSvc *sessionSvc.Service, cfg *config.Config, logger *logrus.Logger) *SubAgentManager {
	return &SubAgentManager{
		agent:      agent,
		sessionSvc: sessionSvc,
		config:     cfg,
		logger:     logger,
		subAgents:  make(map[string]*SubAgentContext),
	}
}

// CreateSubAgent creates a new sub-agent to handle a task.
func (m *SubAgentManager) CreateSubAgent(ctx context.Context, parentSessionID string, task string) (*SubAgentContext, error) {
	// Load parent session to inherit work directory
	parentSession, err := m.sessionSvc.GetSession(parentSessionID)
	if err != nil {
		m.logger.WithError(err).WithField("parent_session_id", parentSessionID).Error("failed to load parent session for sub-agent")
		return nil, fmt.Errorf("failed to load parent session: %w", err)
	}

	// Create a new session for the sub-agent
	subSession, err := m.sessionSvc.CreateSession(parentSession.WorkDir)
	if err != nil {
		m.logger.WithError(err).Error("failed to create sub-agent session")
		return nil, fmt.Errorf("failed to create sub-agent session: %w", err)
	}

	// Create a cancellable context for the sub-agent
	subCtx, cancel := context.WithCancel(ctx)

	subAgentCtx := &SubAgentContext{
		ParentSessionID: parentSessionID,
		SessionID:       subSession.ID,
		Task:            task,
		Status:          "running",
		CreatedAt:       time.Now(),
		cancel:          cancel,
	}

	m.mu.Lock()
	m.subAgents[subSession.ID] = subAgentCtx
	m.mu.Unlock()

	m.logger.WithFields(logrus.Fields{
		"parent_session_id": parentSessionID,
		"sub_session_id":    subSession.ID,
		"task":              task,
	}).Info("sub-agent created")

	// Spawn a goroutine to run the sub-agent
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.WithField("sub_session_id", subSession.ID).Errorf("sub-agent panicked: %v", r)
				m.mu.Lock()
				subAgentCtx.Status = "failed"
				subAgentCtx.Result = fmt.Sprintf("Sub-agent panicked: %v", r)
				m.mu.Unlock()
			}
		}()

		m.logger.WithField("sub_session_id", subSession.ID).Info("sub-agent execution started")

		result, err := m.agent.Run(subCtx, subSession.ID, task, nil)
		m.mu.Lock()
		defer m.mu.Unlock()

		if err != nil {
			if subCtx.Err() != nil {
				subAgentCtx.Status = "failed"
				subAgentCtx.Result = "Sub-agent cancelled"
			} else {
				subAgentCtx.Status = "failed"
				subAgentCtx.Result = fmt.Sprintf("Sub-agent error: %v", err)
			}
			m.logger.WithError(err).WithField("sub_session_id", subSession.ID).Error("sub-agent execution failed")
		} else {
			subAgentCtx.Status = "completed"
			subAgentCtx.Result = result.Content
			m.logger.WithField("sub_session_id", subSession.ID).Info("sub-agent execution completed")
		}
	}()

	return subAgentCtx, nil
}

// GetSubAgentStatus returns the status of a sub-agent.
func (m *SubAgentManager) GetSubAgentStatus(sessionID string) (*SubAgentContext, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ctx, ok := m.subAgents[sessionID]
	if !ok {
		return nil, fmt.Errorf("sub-agent not found: %s", sessionID)
	}

	return ctx, nil
}

// ListSubAgents lists all sub-agents for a parent session.
func (m *SubAgentManager) ListSubAgents(parentSessionID string) []*SubAgentContext {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*SubAgentContext
	for _, sa := range m.subAgents {
		if sa.ParentSessionID == parentSessionID {
			result = append(result, sa)
		}
	}

	return result
}

// CancelSubAgent cancels a running sub-agent.
func (m *SubAgentManager) CancelSubAgent(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, ok := m.subAgents[sessionID]
	if !ok {
		return fmt.Errorf("sub-agent not found: %s", sessionID)
	}

	if ctx.Status != "running" {
		return fmt.Errorf("sub-agent is not running (status: %s)", ctx.Status)
	}

	ctx.cancel()
	ctx.Status = "failed"
	ctx.Result = "Sub-agent cancelled by user"

	m.logger.WithField("sub_session_id", sessionID).Info("sub-agent cancelled")

	return nil
}