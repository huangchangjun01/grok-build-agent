package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Goal represents a tracked goal.
type Goal struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // "pending", "in_progress", "completed", "failed"
	SubGoals    []*Goal   `json:"sub_goals,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GoalManager manages goals for sessions.
type GoalManager struct {
	goals  map[string]map[string]*Goal // sessionID -> goalID -> Goal
	mu     sync.RWMutex
	logger *logrus.Logger
}

// NewGoalManager creates a new GoalManager.
func NewGoalManager(logger *logrus.Logger) *GoalManager {
	return &GoalManager{
		goals:  make(map[string]map[string]*Goal),
		logger: logger,
	}
}

// CreateGoal creates a new goal.
func (m *GoalManager) CreateGoal(sessionID string, description string) *Goal {
	m.mu.Lock()
	defer m.mu.Unlock()

	goal := &Goal{
		ID:          uuid.New().String(),
		SessionID:   sessionID,
		Description: description,
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if m.goals[sessionID] == nil {
		m.goals[sessionID] = make(map[string]*Goal)
	}
	m.goals[sessionID][goal.ID] = goal

	m.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
		"goal_id":    goal.ID,
	}).Info("goal created")

	return goal
}

// UpdateGoal updates a goal's description or status.
func (m *GoalManager) UpdateGoal(sessionID, goalID string, description, status string) (*Goal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionGoals, ok := m.goals[sessionID]
	if !ok {
		return nil, fmt.Errorf("no goals found for session: %s", sessionID)
	}

	goal, ok := sessionGoals[goalID]
	if !ok {
		return nil, fmt.Errorf("goal not found: %s", goalID)
	}

	if description != "" {
		goal.Description = description
	}
	if status != "" {
		goal.Status = status
	}
	goal.UpdatedAt = time.Now()

	m.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
		"goal_id":    goalID,
		"status":     goal.Status,
	}).Info("goal updated")

	return goal, nil
}

// GetGoals returns all goals for a session.
func (m *GoalManager) GetGoals(sessionID string) []*Goal {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionGoals, ok := m.goals[sessionID]
	if !ok {
		return nil
	}

	result := make([]*Goal, 0, len(sessionGoals))
	for _, g := range sessionGoals {
		result = append(result, g)
	}

	return result
}

// PlanGoal decomposes a goal into sub-goals (simple heuristic).
func (m *GoalManager) PlanGoal(goal *Goal) []*Goal {
	m.mu.Lock()
	defer m.mu.Unlock()

	subGoals := m.decompose(goal.Description)

	now := time.Now()
	for _, sg := range subGoals {
		sg.ID = uuid.New().String()
		sg.SessionID = goal.SessionID
		sg.CreatedAt = now
		sg.UpdatedAt = now
		sg.Status = "pending"
	}

	goal.SubGoals = subGoals
	goal.UpdatedAt = now

	m.logger.WithFields(logrus.Fields{
		"session_id":  goal.SessionID,
		"goal_id":     goal.ID,
		"sub_goals":   len(subGoals),
		"description": goal.Description,
	}).Info("goal decomposed into sub-goals")

	return subGoals
}

// decompose is a simple heuristic that splits a goal description into sub-goals.
// It looks for common delimiters and numbered items.
func (m *GoalManager) decompose(description string) []*Goal {
	// Try to split by numbered items: "1. ...", "2. ..."
	lines := strings.Split(description, "\n")
	var items []string
	inNumberedList := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Match lines like "1. ...", "1) ...", "step 1: ..."
		if len(line) > 2 && (line[0] >= '1' && line[0] <= '9') {
			if line[1] == '.' || line[1] == ')' || line[1] == ':' {
				items = append(items, strings.TrimSpace(line[2:]))
				inNumberedList = true
				continue
			}
		}
		// Match "Step 1: ..." or "step 1: ..."
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "step ") && len(line) > 5 {
			if idx := strings.Index(line, ":"); idx > 0 {
				items = append(items, strings.TrimSpace(line[idx+1:]))
				inNumberedList = true
				continue
			}
		}
		// Match "- item" or "* item" or "• item" bullet points
		if (len(line) > 2 && (line[0] == '-' || line[0] == '*')) || strings.HasPrefix(line, "\u2022 ") {
			trimmed := strings.TrimSpace(line[1:])
			if strings.HasPrefix(line, "\u2022 ") {
				trimmed = strings.TrimSpace(line[3:]) // bullet is 3 bytes in UTF-8
			}
			items = append(items, trimmed)
			inNumberedList = true
			continue
		}
	}

	if inNumberedList && len(items) > 1 {
		subGoals := make([]*Goal, 0, len(items))
		for _, item := range items {
			subGoals = append(subGoals, &Goal{Description: item})
		}
		return subGoals
	}

	// Try to split by common conjunction keywords
	keywords := []string{" and ", " then ", " also ", " first ", " next ", " finally "}
	remainder := description
	for _, kw := range keywords {
		parts := strings.Split(remainder, kw)
		if len(parts) > 1 {
			subGoals := make([]*Goal, 0, len(parts))
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					subGoals = append(subGoals, &Goal{Description: part})
				}
			}
			if len(subGoals) > 1 {
				return subGoals
			}
		}
	}

	// If no decomposition is possible, create a single sub-goal with the same description
	return []*Goal{{Description: description}}
}

// SummarizeGoal generates a summary of goal progress.
func (m *GoalManager) SummarizeGoal(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionGoals, ok := m.goals[sessionID]
	if !ok || len(sessionGoals) == 0 {
		return "No goals found for this session."
	}

	var sb strings.Builder
	sb.WriteString("## Goal Summary\n\n")

	total := 0
	completed := 0
	failed := 0
	inProgress := 0
	pending := 0

	for _, goal := range sessionGoals {
		total++
		switch goal.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		case "in_progress":
			inProgress++
		case "pending":
			pending++
		}
	}

	sb.WriteString(fmt.Sprintf("Total: %d | Completed: %d | Failed: %d | In Progress: %d | Pending: %d\n\n",
		total, completed, failed, inProgress, pending))

	for _, goal := range sessionGoals {
		statusIcon := ""
		switch goal.Status {
		case "completed":
			statusIcon = "[✓]"
		case "failed":
			statusIcon = "[✗]"
		case "in_progress":
			statusIcon = "[⋯]"
		case "pending":
			statusIcon = "[ ]"
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", statusIcon, goal.Description))

		if len(goal.SubGoals) > 0 {
			for _, sg := range goal.SubGoals {
				subIcon := ""
				switch sg.Status {
				case "completed":
					subIcon = "    [✓]"
				case "failed":
					subIcon = "    [✗]"
				case "in_progress":
					subIcon = "    [⋯]"
				case "pending":
					subIcon = "    [ ]"
				}
				sb.WriteString(fmt.Sprintf("%s %s\n", subIcon, sg.Description))
			}
		}
	}

	return sb.String()
}

// VerifyGoal checks if a goal is completed.
func (m *GoalManager) VerifyGoal(goal *Goal) bool {
	if goal == nil {
		return false
	}

	if goal.Status == "completed" {
		return true
	}

	if goal.Status == "failed" {
		return false
	}

	// If there are sub-goals, check if all are completed
	if len(goal.SubGoals) > 0 {
		for _, sg := range goal.SubGoals {
			if sg.Status != "completed" {
				return false
			}
		}
		// All sub-goals completed, mark the parent goal as completed
		goal.Status = "completed"
		goal.UpdatedAt = time.Now()
		return true
	}

	return false
}

// StopDetection checks if the agent should stop working on goals.
func (m *GoalManager) StopDetection(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionGoals, ok := m.goals[sessionID]
	if !ok || len(sessionGoals) == 0 {
		return false
	}

	for _, goal := range sessionGoals {
		if goal.Status != "completed" && goal.Status != "failed" {
			// Check sub-goals
			allResolved := true
			if len(goal.SubGoals) > 0 {
				for _, sg := range goal.SubGoals {
					if sg.Status != "completed" && sg.Status != "failed" {
						allResolved = false
						break
					}
				}
			} else {
				allResolved = false
			}
			if !allResolved {
				return false
			}
			// Mark as completed/failed based on sub-goal resolution
			allCompleted := true
			for _, sg := range goal.SubGoals {
				if sg.Status != "completed" {
					allCompleted = false
					break
				}
			}
			if allCompleted {
				goal.Status = "completed"
			} else {
				goal.Status = "failed"
			}
			goal.UpdatedAt = time.Now()
		}
	}

	return true
}