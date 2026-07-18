package session

import (
	"time"
)

// Role represents the role of a message participant.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// Session represents a conversation session.
type Session struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	WorkDir      string                 `json:"work_dir"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	MessageCount int                    `json:"message_count"`
	IsActive     bool                   `json:"is_active"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// Message represents a single message within a session.
type Message struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	Role      Role       `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ToolCall represents a tool invocation within a message.
type ToolCall struct {
	ID        string    `json:"id"`
	MessageID string    `json:"message_id"`
	Name      string    `json:"name"`
	Arguments string    `json:"arguments"`
	Result    string    `json:"result,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Conversation holds a collection of messages with helper methods.
type Conversation struct {
	Messages []Message `json:"messages"`
}

// AddMessage appends a message to the conversation.
func (c *Conversation) AddMessage(msg Message) {
	c.Messages = append(c.Messages, msg)
}

// GetLastN returns the last n messages from the conversation.
// If n is greater than the total number of messages, all messages are returned.
func (c *Conversation) GetLastN(n int) []Message {
	if n <= 0 {
		return nil
	}
	if n > len(c.Messages) {
		n = len(c.Messages)
	}
	return c.Messages[len(c.Messages)-n:]
}

// GetTokenCount returns an estimated token count for the entire conversation.
// This is a simple character-based estimation (~4 chars per token).
func (c *Conversation) GetTokenCount() int {
	total := 0
	for _, msg := range c.Messages {
		total += len(msg.Content) / 4
		for _, tc := range msg.ToolCalls {
			total += len(tc.Name) / 4
			total += len(tc.Arguments) / 4
			total += len(tc.Result) / 4
		}
	}
	return total
}

// Truncate removes messages from the beginning so that the conversation
// stays within the given token limit. It always keeps at least one message.
func (c *Conversation) Truncate(maxTokens int) {
	if maxTokens <= 0 || len(c.Messages) == 0 {
		return
	}
	for c.GetTokenCount() > maxTokens && len(c.Messages) > 1 {
		c.Messages = c.Messages[1:]
	}
}