package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spacexc/go-backend/internal/domain/conversation"
	domain "github.com/spacexc/go-backend/internal/domain/session"
	domaintool "github.com/spacexc/go-backend/internal/domain/tool"
	"github.com/spacexc/go-backend/internal/infrastructure/llm"
	"github.com/spacexc/go-backend/pkg/config"
)

// Agent is the core AI coding agent that orchestrates the LLM interaction loop.
type Agent struct {
	config        *config.Config
	llmClient     *llm.Client
	toolRegistry  domaintool.ToolRegistry
	sessionRepo   domain.SessionRepository
	promptBuilder *PromptBuilder
	compactor     *Compactor
	logger        *logrus.Logger
}

// NewAgent creates a new Agent instance.
func NewAgent(
	cfg *config.Config,
	llmClient *llm.Client,
	toolRegistry domaintool.ToolRegistry,
	sessionRepo domain.SessionRepository,
	logger *logrus.Logger,
) *Agent {
	return &Agent{
		config:        cfg,
		llmClient:     llmClient,
		toolRegistry:  toolRegistry,
		sessionRepo:   sessionRepo,
		promptBuilder: NewPromptBuilder(&cfg.Agent, logger),
		compactor:     NewCompactor(&cfg.Agent, logger),
		logger:        logger,
	}
}

// Run executes the agent loop for a session.
// ctx provides cancellation support.
// sessionID identifies the target session.
// userMessage is the incoming user input.
// streamCh receives real-time text chunks for WebSocket streaming to the client.
// Returns the final assistant response message.
func (a *Agent) Run(
	ctx context.Context,
	sessionID string,
	userMessage string,
	streamCh chan<- string,
) (*domain.Message, error) {
	a.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
		"msg_len":    len(userMessage),
	}).Info("agent run started")

	// Load session
	session, err := a.sessionRepo.GetByID(sessionID)
	if err != nil {
		a.logger.WithError(err).Error("failed to load session")
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	workDir := session.WorkDir

	// Load existing messages
	existingMsgs, err := a.sessionRepo.GetMessages(sessionID, 0, 0)
	if err != nil {
		a.logger.WithError(err).Error("failed to load session messages")
		return nil, fmt.Errorf("failed to load session messages: %w", err)
	}

	// Save the user message to the repository
	userMsg := &domain.Message{
		SessionID: sessionID,
		Role:      domain.RoleUser,
		Content:   userMessage,
		CreatedAt: time.Now(),
	}
	if err := a.sessionRepo.AddMessage(sessionID, userMsg); err != nil {
		a.logger.WithError(err).Error("failed to save user message")
		return nil, fmt.Errorf("failed to save user message: %w", err)
	}

	// Build conversation from existing messages + user message
	conv := &conversation.Conversation{}
	for _, m := range existingMsgs {
		role := string(m.Role)
		content := m.Content
		// For tool messages, include the tool call result
		if m.Role == domain.RoleTool && len(m.ToolCalls) > 0 {
			content = m.ToolCalls[0].Result
		}
		conv.AddMessage(conversation.Message{Role: role, Content: content})
	}
	conv.AddMessage(conversation.Message{Role: string(domain.RoleUser), Content: userMessage})

	a.logger.WithFields(logrus.Fields{
		"session_id":    sessionID,
		"message_count": len(conv.Messages),
		"token_count":   conv.TokenCount,
	}).Debug("conversation built")

	// Determine max turns
	maxTurns := a.config.Agent.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	// Run the agent loop
	result, err := a.agentLoop(ctx, sessionID, workDir, conv, streamCh, maxTurns)
	if err != nil {
		a.logger.WithError(err).Error("agent loop failed")
		return nil, err
	}

	a.logger.WithFields(logrus.Fields{
		"session_id":       sessionID,
		"final_msg_length": len(result.Content),
	}).Info("agent run completed")

	return result, nil
}

// agentLoop is the internal loop: send to LLM -> parse response -> execute tools -> repeat.
func (a *Agent) agentLoop(
	ctx context.Context,
	sessionID string,
	workDir string,
	conv *conversation.Conversation,
	streamCh chan<- string,
	maxTurns int,
) (*domain.Message, error) {
	for turn := 0; turn < maxTurns; turn++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		a.logger.WithFields(logrus.Fields{
			"session_id": sessionID,
			"turn":       turn + 1,
			"max_turns":  maxTurns,
		}).Debug("starting agent loop turn")

		// Check if compaction is needed
		if a.compactor.NeedsCompaction(conv) {
			a.logger.Info("compaction needed, compacting conversation")
			if err := a.compactor.Compact(conv, a.llmClient); err != nil {
				a.logger.WithError(err).Warn("compaction failed, continuing with uncompacted conversation")
			}
		}

		// Build tool definitions from the registry
		toolDefs := a.buildToolDefs()

		// Build messages with system prompt, history, and token limits
		messages := a.promptBuilder.BuildMessages(conv, toolDefs)

		// Prepare the LLM request
		req := &llm.ChatRequest{
			Model:    a.config.LLM.DefaultModel,
			Messages: messages,
			Stream:   true,
			Tools:    toolDefs,
		}

		a.logger.WithFields(logrus.Fields{
			"session_id":    sessionID,
			"turn":          turn + 1,
			"message_count": len(messages),
			"tool_count":    len(toolDefs),
		}).Debug("calling LLM")

		// Call LLM with streaming
		chunkCh, errCh := a.llmClient.ChatStream(ctx, req)

		// Collect streaming response
		var textBuilder strings.Builder
		toolCallAccum := newToolCallAccumulator()
		finishReason := ""

	streamLoop:
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case chunk, ok := <-chunkCh:
				if !ok {
					break streamLoop
				}
				for _, choice := range chunk.Choices {
					// Accumulate text content
					if choice.Delta.Content != "" {
						textBuilder.WriteString(choice.Delta.Content)
						// Send text chunk to the stream channel
						if streamCh != nil {
							select {
							case streamCh <- choice.Delta.Content:
							case <-ctx.Done():
								return nil, ctx.Err()
							default:
								// Non-blocking send to avoid stalling the stream
							}
						}
					}
					// Accumulate tool calls from delta
					if len(choice.Delta.ToolCalls) > 0 {
						toolCallAccum.merge(choice.Delta.ToolCalls)
					}
					// Track finish reason
					if choice.FinishReason != "" {
						finishReason = choice.FinishReason
					}
				}
			case err, ok := <-errCh:
				if ok && err != nil {
					a.logger.WithError(err).Error("LLM stream error")
					return nil, fmt.Errorf("LLM stream error: %w", err)
				}
			}
		}

		textContent := textBuilder.String()
		toolCalls := toolCallAccum.finalize()

		a.logger.WithFields(logrus.Fields{
			"session_id":    sessionID,
			"turn":          turn + 1,
			"text_length":   len(textContent),
			"tool_calls":    len(toolCalls),
			"finish_reason": finishReason,
		}).Debug("LLM response received")

		// Handle tool calls
		if len(toolCalls) > 0 {
			// Save the assistant message with tool calls to the conversation
			assistantMsg := conversation.Message{
				Role:    string(domain.RoleAssistant),
				Content: textContent,
			}
			// Include tool_calls for the LLM to reference
			for _, tc := range toolCalls {
				assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, conversation.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: conversation.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
			conv.AddMessage(assistantMsg)

			// Save to repository
			sessionMsg := &domain.Message{
				SessionID: sessionID,
				Role:      domain.RoleAssistant,
				Content:   textContent,
				CreatedAt: time.Now(),
			}
			if err := a.sessionRepo.AddMessage(sessionID, sessionMsg); err != nil {
				a.logger.WithError(err).Error("failed to save assistant message")
			}

			// Execute the tool calls
			toolResults, err := a.executeToolCalls(ctx, sessionID, workDir, toolCalls, streamCh)
			if err != nil {
				a.logger.WithError(err).Error("tool execution failed")
				// Add error as a tool result
				conv.AddMessage(conversation.Message{
					Role:    string(domain.RoleTool),
					Content: fmt.Sprintf("Error executing tools: %v", err),
				})
				continue
			}

			// Add tool results to the conversation
			for _, tr := range toolResults {
				conv.AddMessage(conversation.Message{
					Role:       tr.Role,
					Content:    tr.Content,
					ToolCallID: tr.ToolCallID,
					Name:       tr.Name,
				})
			}

			// Continue loop for another turn
			continue
		}

		// No tool calls - this is the final response
		if finishReason == "stop" || finishReason == "length" || finishReason == "" {
			// Save the assistant message to the conversation
			assistantMsg := conversation.Message{
				Role:    string(domain.RoleAssistant),
				Content: textContent,
			}
			conv.AddMessage(assistantMsg)

			// Save to repository
			finalMsg := &domain.Message{
				SessionID: sessionID,
				Role:      domain.RoleAssistant,
				Content:   textContent,
				CreatedAt: time.Now(),
			}
			if err := a.sessionRepo.AddMessage(sessionID, finalMsg); err != nil {
				a.logger.WithError(err).Error("failed to save final assistant message")
			}

			return finalMsg, nil
		}

		// Handle other finish reasons (e.g., content_filter)
		a.logger.WithField("finish_reason", finishReason).Warn("unexpected finish reason, ending loop")
		return &domain.Message{
			SessionID: sessionID,
			Role:      domain.RoleAssistant,
			Content:   textContent,
			CreatedAt: time.Now(),
		}, nil
	}

	// Max turns reached
	a.logger.WithField("max_turns", maxTurns).Warn("agent loop reached max turns")
	return &domain.Message{
		SessionID: sessionID,
		Role:      domain.RoleAssistant,
		Content:   "The agent has reached the maximum number of turns. Please refine your request and try again.",
		CreatedAt: time.Now(),
	}, nil
}

// executeToolCalls executes tool calls requested by the LLM.
// It returns chat messages with role "tool" to be added to the conversation.
func (a *Agent) executeToolCalls(
	ctx context.Context,
	sessionID string,
	workDir string,
	toolCalls []llm.ToolCall,
	streamCh chan<- string,
) ([]llm.ChatMessage, error) {
	toolResults := make([]llm.ChatMessage, 0, len(toolCalls))

	for _, tc := range toolCalls {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		a.logger.WithFields(logrus.Fields{
			"session_id": sessionID,
			"tool_name":  tc.Function.Name,
			"tool_id":    tc.ID,
		}).Info("executing tool call")

		// Notify client about tool execution start
		if streamCh != nil {
			status := fmt.Sprintf("\n🔧 Executing tool: %s...\n", tc.Function.Name)
			select {
			case streamCh <- status:
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Look up the tool in the registry
		t, ok := a.toolRegistry.Get(tc.Function.Name)
		if !ok {
			errMsg := fmt.Sprintf("Tool not found: %s", tc.Function.Name)
			a.logger.WithField("tool_name", tc.Function.Name).Warn(errMsg)
			toolResults = append(toolResults, llm.ChatMessage{
				Role:       "tool",
				Content:    errMsg,
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
			})
			continue
		}

		// Parse arguments from JSON string
		var params map[string]interface{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
				errMsg := fmt.Sprintf("Failed to parse tool arguments: %v", err)
				a.logger.WithError(err).WithField("tool_name", tc.Function.Name).Warn(errMsg)
				toolResults = append(toolResults, llm.ChatMessage{
					Role:       "tool",
					Content:    errMsg,
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
				})
				continue
			}
		}

		// Create tool context and execute
		toolCtx := domaintool.ToolContext{
			SessionID: sessionID,
			WorkDir:   workDir,
			Params:    params,
		}

		result, err := t.Execute(toolCtx)
		if err != nil {
			a.logger.WithError(err).WithField("tool_name", tc.Function.Name).Error("tool execution failed")
			toolResults = append(toolResults, llm.ChatMessage{
				Role:       "tool",
				Content:    fmt.Sprintf("Tool execution error: %v", err),
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
			})
			continue
		}

		// Build result content
		resultContent := result.Output
		if !result.Success && result.Error != "" {
			resultContent = fmt.Sprintf("Error: %s\nOutput: %s", result.Error, result.Output)
		}

		// Notify client about tool execution result
		if streamCh != nil {
			status := fmt.Sprintf("✅ Tool %s completed.\n", tc.Function.Name)
			select {
			case streamCh <- status:
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Save tool result as a session message
		toolMsg := &domain.Message{
			SessionID: sessionID,
			Role:      domain.RoleTool,
			Content:   resultContent,
			ToolCalls: []domain.ToolCall{
				{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
					Result:    resultContent,
					Status:    "completed",
					CreatedAt: time.Now(),
				},
			},
			CreatedAt: time.Now(),
		}
		if err := a.sessionRepo.AddMessage(sessionID, toolMsg); err != nil {
			a.logger.WithError(err).Warn("failed to save tool result message")
		}

		toolResults = append(toolResults, llm.ChatMessage{
			Role:       "tool",
			Content:    resultContent,
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
		})
	}

	return toolResults, nil
}

// updateConversation saves messages to the repository.
func (a *Agent) updateConversation(sessionID string, messages []*domain.Message) error {
	for _, msg := range messages {
		if err := a.sessionRepo.AddMessage(sessionID, msg); err != nil {
			return fmt.Errorf("failed to add message: %w", err)
		}
	}
	return nil
}

// buildToolDefs constructs OpenAI-format tool definitions from the registry.
func (a *Agent) buildToolDefs() []llm.ToolDef {
	tools := a.toolRegistry.List()
	defs := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}

// toolCallAccumulator accumulates streaming tool call deltas by index.
type toolCallAccumulator struct {
	calls []llm.ToolCall
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{}
}

// merge merges incoming delta tool calls into the accumulator.
// Tool calls are accumulated by their index position.
func (a *toolCallAccumulator) merge(deltas []llm.ToolCall) {
	for _, delta := range deltas {
		idx := delta.Index
		// Ensure the slice has enough capacity
		for idx >= len(a.calls) {
			a.calls = append(a.calls, llm.ToolCall{})
		}
		// Merge ID
		if delta.ID != "" {
			a.calls[idx].ID = delta.ID
		}
		// Merge type
		if delta.Type != "" {
			a.calls[idx].Type = delta.Type
		}
		// Merge function name
		if delta.Function.Name != "" {
			a.calls[idx].Function.Name = delta.Function.Name
		}
		// Accumulate function arguments (streamed in chunks)
		if delta.Function.Arguments != "" {
			a.calls[idx].Function.Arguments += delta.Function.Arguments
		}
	}
}

// finalize returns the accumulated tool calls, filtering out empty entries.
func (a *toolCallAccumulator) finalize() []llm.ToolCall {
	result := make([]llm.ToolCall, 0, len(a.calls))
	for _, tc := range a.calls {
		if tc.ID != "" || tc.Function.Name != "" {
			result = append(result, tc)
		}
	}
	return result
}