package agent

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spacexc/go-backend/internal/domain/conversation"
	"github.com/spacexc/go-backend/internal/infrastructure/llm"
	"github.com/spacexc/go-backend/pkg/config"
)

const SystemPromptTemplate = `You are an interactive AI coding agent. You are running in a sandboxed environment with access to tools that allow you to read, write, and execute code.

## Your Capabilities
- Read and write files on the filesystem
- Execute shell commands in a sandboxed environment
- Search codebases and directories
- Analyze code and provide suggestions
- Make edits to existing files

## Coding Best Practices
- Always read relevant files before making changes to understand the existing code
- Follow the existing code style, patterns, and conventions in the project
- Never assume a library is available — check package.json / go.mod / requirements.txt first
- Only make changes directly required by the task — avoid over-engineering
- Do not add comments to code you didn't change
- Use proper error handling in code you write
- Always follow security best practices

## Tool Usage
- When you need to perform a task, use the appropriate tool
- You may call multiple tools in parallel when they are independent
- Read tool results carefully before proceeding
- If a tool call fails, analyze the error and try a different approach

## Response Guidelines
- Think step by step before acting
- Explain your reasoning briefly before making changes
- When you complete a task, provide a concise summary of what was done
- If you encounter an error you cannot resolve, explain the issue clearly`

// PromptBuilder builds prompts for the LLM.
type PromptBuilder struct {
	config *config.AgentConfig
	logger *logrus.Logger
}

// NewPromptBuilder creates a new PromptBuilder.
func NewPromptBuilder(cfg *config.AgentConfig, logger *logrus.Logger) *PromptBuilder {
	return &PromptBuilder{
		config: cfg,
		logger: logger,
	}
}

// BuildSystemPrompt builds the system prompt with tool descriptions appended.
func (b *PromptBuilder) BuildSystemPrompt(toolDefs []llm.ToolDef) string {
	var sb strings.Builder
	sb.WriteString(SystemPromptTemplate)

	if len(toolDefs) > 0 {
		sb.WriteString("\n\n## Available Tools\n\n")
		for _, td := range toolDefs {
			sb.WriteString(fmt.Sprintf("### %s\n", td.Function.Name))
			sb.WriteString(fmt.Sprintf("%s\n\n", td.Function.Description))
		}
	}

	return sb.String()
}

// BuildMessages converts a Conversation to OpenAI-format messages.
// It prepends the system prompt, includes conversation history, and applies
// token limits based on the configured context window.
func (b *PromptBuilder) BuildMessages(conv *conversation.Conversation, toolDefs []llm.ToolDef) []llm.ChatMessage {
	systemPrompt := b.BuildSystemPrompt(toolDefs)
	messages := make([]llm.ChatMessage, 0, len(conv.Messages)+1)

	// Always start with the system prompt
	messages = append(messages, llm.ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	contextWindow := b.config.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 32000
	}

	// Estimate total tokens and truncate if needed
	totalTokens := conversation.EstimateTokens(systemPrompt)
	var msgsToInclude []conversation.Message

	// Walk backwards to determine which messages fit within the context window
	// Reserve ~20% for the response
	availableTokens := int(float64(contextWindow) * 0.8)

	for i := len(conv.Messages) - 1; i >= 0; i-- {
		msgTokens := conversation.EstimateTokens(conv.Messages[i].Content)
		if totalTokens+msgTokens > availableTokens {
			break
		}
		totalTokens += msgTokens
		msgsToInclude = append([]conversation.Message{conv.Messages[i]}, msgsToInclude...)
	}

	for _, msg := range msgsToInclude {
		messages = append(messages, llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	b.logger.WithFields(logrus.Fields{
		"total_messages":   len(messages),
		"history_messages": len(msgsToInclude),
		"estimated_tokens": totalTokens,
		"context_window":   contextWindow,
	}).Debug("built messages for LLM request")

	return messages
}