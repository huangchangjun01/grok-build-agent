package telemetry

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// Metrics collects operational metrics.
type Metrics struct {
	TotalRequests  int64            `json:"total_requests"`
	TotalTokensIn  int64            `json:"total_tokens_in"`
	TotalTokensOut int64            `json:"total_tokens_out"`
	TotalToolCalls int64            `json:"total_tool_calls"`
	TotalErrors    int64            `json:"total_errors"`
	AverageLatency float64          `json:"average_latency_ms"`
	SessionsActive int64            `json:"sessions_active"`
	SessionMetrics map[string]*SessionMetric `json:"-"`
	mu             sync.RWMutex
	logger         *logrus.Logger
	latencySum     float64 // total accumulated latency for average calculation
	latencyCount   int64   // total number of latency samples
}

// SessionMetric tracks per-session metrics.
type SessionMetric struct {
	SessionID    string    `json:"session_id"`
	MessageCount int64     `json:"message_count"`
	TokenCount   int64     `json:"token_count"`
	ToolCalls    int64     `json:"tool_calls"`
	Errors       int64     `json:"errors"`
	CreatedAt    time.Time `json:"created_at"`
	LastActive   time.Time `json:"last_active"`
}

// Telemetry collects and reports metrics.
type Telemetry struct {
	metrics *Metrics
	logger  *logrus.Logger
}

// NewTelemetry creates a new Telemetry instance.
func NewTelemetry(logger *logrus.Logger) *Telemetry {
	return &Telemetry{
		metrics: &Metrics{
			SessionMetrics: make(map[string]*SessionMetric),
			logger:         logger,
		},
		logger: logger,
	}
}

// RecordRequest records a request with its latency.
func (t *Telemetry) RecordRequest(latency time.Duration) {
	atomic.AddInt64(&t.metrics.TotalRequests, 1)

	latencyMs := float64(latency.Microseconds()) / 1000.0
	t.metrics.mu.Lock()
	t.metrics.latencySum += latencyMs
	t.metrics.latencyCount++
	t.metrics.AverageLatency = t.metrics.latencySum / float64(t.metrics.latencyCount)
	t.metrics.mu.Unlock()

	t.logger.WithFields(logrus.Fields{
		"latency_ms":  latencyMs,
		"total_count": atomic.LoadInt64(&t.metrics.TotalRequests),
	}).Debug("telemetry: request recorded")
}

// RecordTokens records token usage.
func (t *Telemetry) RecordTokens(tokensIn, tokensOut int64) {
	atomic.AddInt64(&t.metrics.TotalTokensIn, tokensIn)
	atomic.AddInt64(&t.metrics.TotalTokensOut, tokensOut)

	t.logger.WithFields(logrus.Fields{
		"tokens_in":  tokensIn,
		"tokens_out": tokensOut,
	}).Debug("telemetry: tokens recorded")
}

// RecordToolCall records a tool call.
func (t *Telemetry) RecordToolCall() {
	atomic.AddInt64(&t.metrics.TotalToolCalls, 1)

	t.logger.WithField(
		"total_tool_calls", atomic.LoadInt64(&t.metrics.TotalToolCalls),
	).Debug("telemetry: tool call recorded")
}

// RecordError records an error.
func (t *Telemetry) RecordError(err error) {
	atomic.AddInt64(&t.metrics.TotalErrors, 1)

	t.logger.WithFields(logrus.Fields{
		"error":       err.Error(),
		"total_errors": atomic.LoadInt64(&t.metrics.TotalErrors),
	}).Debug("telemetry: error recorded")
}

// TrackSession starts tracking a session.
func (t *Telemetry) TrackSession(sessionID string) {
	t.metrics.mu.Lock()
	defer t.metrics.mu.Unlock()

	if _, exists := t.metrics.SessionMetrics[sessionID]; !exists {
		t.metrics.SessionMetrics[sessionID] = &SessionMetric{
			SessionID:  sessionID,
			CreatedAt:  time.Now(),
			LastActive: time.Now(),
		}
		atomic.AddInt64(&t.metrics.SessionsActive, 1)
		t.logger.WithField("session_id", sessionID).Info("telemetry: session tracking started")
	}
}

// UntrackSession stops tracking a session.
func (t *Telemetry) UntrackSession(sessionID string) {
	t.metrics.mu.Lock()
	defer t.metrics.mu.Unlock()

	if _, exists := t.metrics.SessionMetrics[sessionID]; exists {
		delete(t.metrics.SessionMetrics, sessionID)
		atomic.AddInt64(&t.metrics.SessionsActive, -1)
		t.logger.WithField("session_id", sessionID).Info("telemetry: session tracking stopped")
	}
}

// UpdateSessionMetrics updates metrics for a session.
func (t *Telemetry) UpdateSessionMetrics(sessionID string, messages, tokens, toolCalls, errors int64) {
	t.metrics.mu.Lock()
	defer t.metrics.mu.Unlock()

	sm, exists := t.metrics.SessionMetrics[sessionID]
	if !exists {
		sm = &SessionMetric{
			SessionID: sessionID,
			CreatedAt: time.Now(),
		}
		t.metrics.SessionMetrics[sessionID] = sm
		atomic.AddInt64(&t.metrics.SessionsActive, 1)
	}

	sm.MessageCount += messages
	sm.TokenCount += tokens
	sm.ToolCalls += toolCalls
	sm.Errors += errors
	sm.LastActive = time.Now()

	t.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
		"messages":   sm.MessageCount,
		"tokens":     sm.TokenCount,
		"tool_calls": sm.ToolCalls,
		"errors":     sm.Errors,
	}).Debug("telemetry: session metrics updated")
}

// GetMetrics returns current metrics.
func (t *Telemetry) GetMetrics() *Metrics {
	t.metrics.mu.RLock()
	defer t.metrics.mu.RUnlock()

	// Return a copy with atomic reads for the atomic counters
	return &Metrics{
		TotalRequests:  atomic.LoadInt64(&t.metrics.TotalRequests),
		TotalTokensIn:  atomic.LoadInt64(&t.metrics.TotalTokensIn),
		TotalTokensOut: atomic.LoadInt64(&t.metrics.TotalTokensOut),
		TotalToolCalls: atomic.LoadInt64(&t.metrics.TotalToolCalls),
		TotalErrors:    atomic.LoadInt64(&t.metrics.TotalErrors),
		AverageLatency: t.metrics.AverageLatency,
		SessionsActive: atomic.LoadInt64(&t.metrics.SessionsActive),
		SessionMetrics: t.metrics.SessionMetrics,
		mu:             sync.RWMutex{},
		logger:         t.metrics.logger,
		latencySum:     t.metrics.latencySum,
		latencyCount:   t.metrics.latencyCount,
	}
}

// GetSessionMetrics returns metrics for a specific session.
func (t *Telemetry) GetSessionMetrics(sessionID string) *SessionMetric {
	t.metrics.mu.RLock()
	defer t.metrics.mu.RUnlock()

	sm, ok := t.metrics.SessionMetrics[sessionID]
	if !ok {
		return nil
	}

	// Return a copy
	return &SessionMetric{
		SessionID:    sm.SessionID,
		MessageCount: sm.MessageCount,
		TokenCount:   sm.TokenCount,
		ToolCalls:    sm.ToolCalls,
		Errors:       sm.Errors,
		CreatedAt:    sm.CreatedAt,
		LastActive:   sm.LastActive,
	}
}

// LogSummary logs a summary of current metrics.
func (t *Telemetry) LogSummary() {
	metrics := t.GetMetrics()

	t.logger.WithFields(logrus.Fields{
		"total_requests":   metrics.TotalRequests,
		"total_tokens_in":  metrics.TotalTokensIn,
		"total_tokens_out": metrics.TotalTokensOut,
		"total_tool_calls": metrics.TotalToolCalls,
		"total_errors":     metrics.TotalErrors,
		"average_latency":  metrics.AverageLatency,
		"sessions_active":  metrics.SessionsActive,
		"session_count":    len(metrics.SessionMetrics),
	}).Info("telemetry summary")
}