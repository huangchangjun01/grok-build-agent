package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

const (
	// defaultMaxRepeat is the default max number of times the same action can repeat
	defaultMaxRepeat = 5
	// defaultRecentActionCount is the number of recent actions to track
	defaultRecentActionCount = 20
)

// LazinessDetector detects when the agent is stuck in a loop.
type LazinessDetector struct {
	recentActions []string // last N tool calls
	maxRepeat     int      // max times the same action can repeat
	logger        *logrus.Logger
	mu            sync.Mutex
}

// NewLazinessDetector creates a new LazinessDetector.
func NewLazinessDetector(logger *logrus.Logger) *LazinessDetector {
	return &LazinessDetector{
		recentActions: make([]string, 0, defaultRecentActionCount),
		maxRepeat:     defaultMaxRepeat,
		logger:        logger,
	}
}

// RecordAction records a tool call action.
func (d *LazinessDetector) RecordAction(toolName string, args string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry := fmt.Sprintf("%s:%s", toolName, args)

	// Append and maintain a fixed-size circular buffer
	if len(d.recentActions) >= defaultRecentActionCount {
		// Shift left by one to make room
		copy(d.recentActions, d.recentActions[1:])
		d.recentActions[len(d.recentActions)-1] = entry
	} else {
		d.recentActions = append(d.recentActions, entry)
	}

	d.logger.WithFields(logrus.Fields{
		"tool":    toolName,
		"history": len(d.recentActions),
	}).Debug("laziness detector: recorded action")
}

// IsLazy checks if the agent appears to be stuck in a loop.
// Returns true if the same action has been repeated more than maxRepeat times.
func (d *LazinessDetector) IsLazy() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.recentActions) < d.maxRepeat {
		return false
	}

	// Check the last maxRepeat entries for repetition
	last := d.recentActions[len(d.recentActions)-1]
	if last == "" {
		return false
	}

	count := 0
	for i := len(d.recentActions) - 1; i >= 0; i-- {
		if d.recentActions[i] == last {
			count++
		} else {
			break
		}
	}

	return count >= d.maxRepeat
}

// GetIntervention returns a prompt to break the agent out of the loop.
func (d *LazinessDetector) GetIntervention() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.recentActions) == 0 {
		return ""
	}

	lastActions := d.recentActions
	if len(lastActions) > 5 {
		lastActions = lastActions[len(lastActions)-5:]
	}

	var sb strings.Builder
	sb.WriteString("You appear to be stuck in a repetitive loop. ")
	sb.WriteString("Please take a different approach:\n\n")
	sb.WriteString("1. Analyze what has been tried so far and why it hasn't worked\n")
	sb.WriteString("2. Consider a completely different strategy or tool\n")
	sb.WriteString("3. If you're repeatedly trying the same thing, stop and ask for clarification\n")
	sb.WriteString("4. Break the problem into smaller, different steps\n\n")
	sb.WriteString("Recent actions that appear to be repeating:\n")
	for _, action := range lastActions {
		sb.WriteString(fmt.Sprintf("- %s\n", action))
	}

	return sb.String()
}

// Reset clears the action history.
func (d *LazinessDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.recentActions = d.recentActions[:0]
	d.logger.Debug("laziness detector: reset")
}