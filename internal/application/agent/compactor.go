package agent

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spacexc/grok-build/internal/domain/conversation"
	"github.com/spacexc/grok-build/internal/infrastructure/llm"
	"github.com/spacexc/grok-build/pkg/config"
)

const (
	// Keep the most recent messages after compaction
	keepRecentMessages = 10
	// Summary prompt template
	summaryPrompt = "Summarize the following conversation between a user and an AI coding assistant. " +
		"Focus on what was discussed, what tasks were requested, what changes were made, and any important decisions. " +
		"Keep the summary concise but comprehensive. Here is the conversation:\n\n%s"
)

// Compactor compacts conversations to fit within token limits.
type Compactor struct {
	config *config.AgentConfig
	logger *logrus.Logger
}

// NewCompactor creates a new Compactor.
func NewCompactor(cfg *config.AgentConfig, logger *logrus.Logger) *Compactor {
	return &Compactor{
		config: cfg,
		logger: logger,
	}
}

// NeedsCompaction checks if the conversation exceeds the compaction threshold.
// Compaction is needed when the estimated token count exceeds
// compaction_threshold * context_window.
func (c *Compactor) NeedsCompaction(conv *conversation.Conversation) bool {
	if conv == nil || len(conv.Messages) == 0 {
		return false
	}

	contextWindow := c.config.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 32000
	}

	threshold := c.config.CompactionThreshold
	if threshold <= 0 {
		threshold = 0.7
	}

	limit := int(float64(contextWindow) * threshold)

	needed := conv.TokenCount > limit
	if needed {
		c.logger.WithFields(logrus.Fields{
			"token_count":    conv.TokenCount,
			"threshold":      limit,
			"context_window": contextWindow,
		}).Debug("compaction needed")
	}

	return needed
}

// Compact compacts the conversation by summarizing older messages.
// It keeps the most recent N messages and summarizes the rest into a single
// system-like message that captures the context of the earlier conversation.
func (c *Compactor) Compact(conv *conversation.Conversation, llmClient *llm.Client) error {
	if conv == nil || len(conv.Messages) <= keepRecentMessages {
		return nil
	}

	c.logger.WithField("message_count", len(conv.Messages)).Info("compacting conversation")

	// Split messages: older ones to summarize, recent ones to keep
	splitIdx := len(conv.Messages) - keepRecentMessages
	olderMessages := conv.Messages[:splitIdx]
	recentMessages := conv.Messages[splitIdx:]

	// Build the text to summarize
	var conversationText string
	for _, msg := range olderMessages {
		conversationText += fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content)
	}

	summaryReq := &llm.ChatRequest{
		Model: llmClient.Config().DefaultModel,
		Messages: []llm.ChatMessage{
			{
				Role:    "system",
				Content: "You are a helpful assistant that summarizes conversations concisely.",
			},
			{
				Role:    "user",
				Content: fmt.Sprintf(summaryPrompt, conversationText),
			},
		},
		Stream: false,
	}

	resp, err := llmClient.Chat(context.Background(), summaryReq)
	if err != nil {
		return fmt.Errorf("failed to summarize conversation: %w", err)
	}

	if len(resp.Choices) == 0 {
		return fmt.Errorf("no response from LLM for compaction summary")
	}

	summary := resp.Choices[0].Message.Content

	// Rebuild the conversation: summary as a system message + recent messages
	newMessages := make([]conversation.Message, 0, len(recentMessages)+1)
	newMessages = append(newMessages, conversation.Message{
		Role:    "system",
		Content: fmt.Sprintf("[Previous conversation summary]\n%s\n\n[Recent conversation continues below]", summary),
	})

	for _, msg := range recentMessages {
		newMessages = append(newMessages, msg)
	}

	conv.Messages = newMessages

	// Recalculate token count
	conv.TokenCount = 0
	for _, msg := range conv.Messages {
		conv.TokenCount += conversation.EstimateTokens(msg.Content)
	}

	c.logger.WithFields(logrus.Fields{
		"new_message_count": len(conv.Messages),
		"new_token_count":   conv.TokenCount,
	}).Info("conversation compacted")

	return nil
}