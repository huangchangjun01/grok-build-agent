package conversation

import (
	"encoding/json"
)

// Message represents a message in the conversation domain.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Conversation holds a collection of messages with token tracking.
type Conversation struct {
	Messages   []Message `json:"messages"`
	TokenCount int       `json:"token_count"`
}

// AddMessage appends a message and updates the token count.
func (c *Conversation) AddMessage(msg Message) {
	c.Messages = append(c.Messages, msg)
	c.TokenCount += EstimateTokens(msg.Content)
}

// GetMessages returns all messages in the conversation.
func (c *Conversation) GetMessages() []Message {
	return c.Messages
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

// TruncateToTokenLimit removes messages from the beginning until the
// conversation fits within the specified token limit. It always preserves
// at least one message.
func (c *Conversation) TruncateToTokenLimit(maxTokens int) {
	if maxTokens <= 0 || len(c.Messages) == 0 {
		return
	}
	for c.TokenCount > maxTokens && len(c.Messages) > 1 {
		removed := c.Messages[0]
		c.Messages = c.Messages[1:]
		c.TokenCount -= EstimateTokens(removed.Content)
	}
}

// openaiMessage is the internal structure for OpenAI-compatible message format.
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToOpenAIFormat converts the conversation to an OpenAI API-compatible
// message array format.
func (c *Conversation) ToOpenAIFormat() ([]byte, error) {
	msgs := make([]openaiMessage, len(c.Messages))
	for i, m := range c.Messages {
		msgs[i] = openaiMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	return json.Marshal(msgs)
}

// EstimateTokens provides a simple character-based token estimation.
// A rough heuristic: ~4 characters per token.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}