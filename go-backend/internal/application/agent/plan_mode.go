package agent

import (
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// PlanModeState tracks the plan mode state for a session.
type PlanModeState struct {
	Enabled   bool      `json:"enabled"`
	Plan      string    `json:"plan"`
	Approved  bool      `json:"approved"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlanModeManager manages plan mode for sessions.
type PlanModeManager struct {
	states map[string]*PlanModeState // sessionID -> state
	mu     sync.RWMutex
	logger *logrus.Logger
}

// NewPlanModeManager creates a new PlanModeManager.
func NewPlanModeManager(logger *logrus.Logger) *PlanModeManager {
	return &PlanModeManager{
		states: make(map[string]*PlanModeState),
		logger: logger,
	}
}

// EnterPlanMode enters plan mode for a session.
func (m *PlanModeManager) EnterPlanMode(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[sessionID]
	if !exists {
		state = &PlanModeState{}
		m.states[sessionID] = state
	}

	state.Enabled = true
	state.UpdatedAt = time.Now()

	m.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
	}).Info("entered plan mode")

	return nil
}

// ExitPlanMode exits plan mode for a session.
func (m *PlanModeManager) ExitPlanMode(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[sessionID]
	if !exists {
		return errors.New("session not in plan mode")
	}

	state.Enabled = false
	state.UpdatedAt = time.Now()

	m.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
	}).Info("exited plan mode")

	return nil
}

// IsInPlanMode checks if a session is in plan mode.
func (m *PlanModeManager) IsInPlanMode(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[sessionID]
	if !exists {
		return false
	}

	return state.Enabled
}

// SetPlan sets the plan for a session.
func (m *PlanModeManager) SetPlan(sessionID string, plan string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[sessionID]
	if !exists {
		state = &PlanModeState{}
		m.states[sessionID] = state
	}

	state.Plan = plan
	state.UpdatedAt = time.Now()

	m.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
		"plan_len":   len(plan),
	}).Debug("plan set")

	return nil
}

// GetPlan gets the plan for a session.
func (m *PlanModeManager) GetPlan(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[sessionID]
	if !exists {
		return ""
	}

	return state.Plan
}

// ApprovePlan approves the plan for execution.
func (m *PlanModeManager) ApprovePlan(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[sessionID]
	if !exists {
		return errors.New("session not in plan mode")
	}

	state.Approved = true
	state.UpdatedAt = time.Now()

	m.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
	}).Info("plan approved")

	return nil
}

// IsApproved checks if the plan is approved.
func (m *PlanModeManager) IsApproved(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[sessionID]
	if !exists {
		return false
	}

	return state.Approved
}