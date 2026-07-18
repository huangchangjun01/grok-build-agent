package agent

import "github.com/spacexc/grok-build/internal/domain/session"

// AgentConfig holds the configuration for an agent.
type AgentConfig struct {
	Model         string  `json:"model"`
	Temperature   float64 `json:"temperature"`
	TopP          float64 `json:"top_p"`
	MaxTokens     int     `json:"max_tokens"`
	ContextWindow int     `json:"context_window"`
	MaxTurns      int     `json:"max_turns"`
	SystemPrompt  string  `json:"system_prompt"`
}

// AgentState represents the current state of an agent.
type AgentState string

const (
	StateIdle      AgentState = "idle"
	StateThinking  AgentState = "thinking"
	StateExecuting AgentState = "executing"
	StateWaiting   AgentState = "waiting"
	StateError     AgentState = "error"
)

// AgentContext bundles the runtime context for an agent execution.
type AgentContext struct {
	SessionID    string                `json:"session_id"`
	Config       AgentConfig           `json:"config"`
	State        AgentState            `json:"state"`
	Conversation session.Conversation  `json:"conversation"`
	WorkDir      string                `json:"work_dir"`
}