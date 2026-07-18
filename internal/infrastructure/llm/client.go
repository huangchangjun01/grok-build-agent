package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spacexc/grok-build/pkg/config"
)

const (
	maxRetries      = 3
	baseBackoff     = 1 * time.Second
	completionsPath = "/chat/completions"
)

// Client wraps the MiniMax OpenAI-compatible API
type Client struct {
	config     *config.LLMConfig
	httpClient *http.Client
	logger     *logrus.Logger
}

// ChatRequest represents an OpenAI-compatible chat completion request
type ChatRequest struct {
	Model       string          `json:"model"`
	Messages    []ChatMessage   `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream"`
	Tools       []ToolDef       `json:"tools,omitempty"`
	ToolChoice  string          `json:"tool_choice,omitempty"`
}

// ChatMessage represents a message in the chat
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolDef represents a function tool definition
type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef represents a function definition
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall represents a tool call from the LLM
type ToolCall struct {
	Index    int          `json:"index,omitempty"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	Delta        ChatMessage `json:"delta,omitempty"`
	FinishReason string      `json:"finish_reason"`
}

// StreamChunk represents a streaming response chunk
type StreamChunk struct {
	ID      string         `json:"id"`
	Choices []StreamChoice `json:"choices"`
}

// StreamChoice represents a streaming choice
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        ChatMessage `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

// Config returns the LLM configuration.
func (c *Client) Config() *config.LLMConfig {
	return c.config
}

// NewClient creates a new MiniMax LLM client
func NewClient(cfg *config.LLMConfig, logger *logrus.Logger) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		logger: logger,
	}
}

// buildURL constructs the full API endpoint URL
func (c *Client) buildURL() string {
	base := strings.TrimRight(c.config.BaseURL, "/")
	return base + completionsPath
}

// Chat sends a chat completion request (non-streaming)
func (c *Client) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"model":    req.Model,
		"messages": len(req.Messages),
	}).Debug("sending chat completion request")

	var resp *ChatResponse
	err = c.doWithRetry(ctx, func() error {
		var attemptErr error
		resp, attemptErr = c.sendChatRequest(ctx, body)
		return attemptErr
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// ChatStream sends a streaming chat completion request, returns a channel of StreamChunk
func (c *Client) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, <-chan error) {
	chunkCh := make(chan StreamChunk, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(chunkCh)
		defer close(errCh)

		req.Stream = true

		body, err := json.Marshal(req)
		if err != nil {
			errCh <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		c.logger.WithFields(logrus.Fields{
			"model":    req.Model,
			"messages": len(req.Messages),
		}).Debug("starting streaming chat completion")

		// Retry on initial connection errors only
		var httpResp *http.Response
		err = c.doWithRetry(ctx, func() error {
			var attemptErr error
			httpResp, attemptErr = c.sendStreamRequest(ctx, body)
			return attemptErr
		})
		if err != nil {
			errCh <- err
			return
		}
		defer httpResp.Body.Close()

		if err := c.parseSSE(ctx, httpResp.Body, chunkCh); err != nil {
			errCh <- err
		}
	}()

	return chunkCh, errCh
}

// sendChatRequest sends a non-streaming request and returns the parsed response
func (c *Client) sendChatRequest(ctx context.Context, body []byte) (*ChatResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(httpReq)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp ChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &resp, nil
}

// sendStreamRequest sends a streaming request and returns the raw HTTP response
func (c *Client) sendStreamRequest(ctx context.Context, body []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, string(respBody))
	}

	return httpResp, nil
}

// setHeaders sets common headers on the HTTP request
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
}

// parseSSE reads SSE events from the reader and sends chunks to the channel
func (c *Client) parseSSE(ctx context.Context, reader io.Reader, chunkCh chan<- StreamChunk) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end marker
		if data == "[DONE]" {
			c.logger.Debug("stream completed")
			return nil
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			c.logger.WithError(err).WithField("data", data).Warn("failed to parse SSE chunk")
			continue
		}

		select {
		case chunkCh <- chunk:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("SSE scanner error: %w", err)
	}

	return nil
}

// doWithRetry executes the given function with retry logic and exponential backoff
func (c *Client) doWithRetry(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Do not retry on context cancellation or permanent errors
		if errors.Is(lastErr, context.Canceled) || errors.Is(lastErr, context.DeadlineExceeded) {
			return lastErr
		}

		backoff := baseBackoff * time.Duration(1<<attempt)
		c.logger.WithError(lastErr).WithFields(logrus.Fields{
			"attempt": attempt + 1,
			"backoff": backoff,
		}).Warn("chat request failed, retrying")

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}

	return fmt.Errorf("all %d retries exhausted: %w", maxRetries, lastErr)
}