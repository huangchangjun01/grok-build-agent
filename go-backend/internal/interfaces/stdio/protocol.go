// Package stdio implements the Leader/Stdio protocol for the Grok Build agent.
// Wire format: 4-byte big-endian length prefix + JSON payload (max 64MB).
package stdio

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const MaxMessageSize = 64 * 1024 * 1024 // 64MB

// ReadFrame reads a length-prefixed frame from the reader.
func ReadFrame(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("read frame length: %w", err)
	}
	length := binary.BigEndian.Uint32(lenBuf[:])
	if length > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max: %d)", length, MaxMessageSize)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read frame data: %w", err)
	}
	return data, nil
}

// WriteFrame writes a length-prefixed frame to the writer.
func WriteFrame(w io.Writer, data []byte) error {
	length := uint32(len(data))
	if length > MaxMessageSize {
		return fmt.Errorf("message too large: %d bytes (max: %d)", length, MaxMessageSize)
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], length)
	if _, err := w.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("write frame length: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write frame data: %w", err)
	}
	return nil
}

// ReadMessage reads and unmarshals a length-prefixed JSON message.
func ReadMessage(r io.Reader, v interface{}) error {
	data, err := ReadFrame(r)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal message: %w", err)
	}
	return nil
}

// WriteMessage marshals and writes a length-prefixed JSON message.
func WriteMessage(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return WriteFrame(w, data)
}

// --- Leader Protocol Message Types ---

// ClientMode determines how the leader handles communication.
type ClientMode string

const (
	ClientModeStdio    ClientMode = "stdio"
	ClientModeHeadless ClientMode = "headless"
)

// ClientCapabilities reported during registration.
type ClientCapabilities struct {
	YoloMode      bool    `json:"yolo_mode,omitempty"`
	AutoMode      bool    `json:"auto_mode,omitempty"`
	DefaultModel  *string `json:"default_model,omitempty"`
	CodeNavEnabled bool   `json:"code_nav_enabled,omitempty"`
	Terminal      bool    `json:"terminal,omitempty"`
	FSRead        bool    `json:"fs_read,omitempty"`
	FSWrite       bool    `json:"fs_write,omitempty"`
}

// ClientMessage sent from client to server.
type ClientMessage struct {
	Type string `json:"type"`

	// Register fields
	ClientType   string             `json:"client_type,omitempty"`
	Mode         ClientMode         `json:"mode,omitempty"`
	Capabilities ClientCapabilities `json:"capabilities,omitempty"`

	// Acp fields
	Payload string `json:"payload,omitempty"`

	// Control fields
	RequestID string          `json:"request_id,omitempty"`
	Command   json.RawMessage `json:"command,omitempty"`
}

// ServerMessage sent from server to client.
type ServerMessage struct {
	Type string `json:"type"`

	// Registered fields
	ClientID              uint64 `json:"client_id,omitempty"`
	Ready                 bool   `json:"ready"`
	LeaderProtocolVersion *uint32 `json:"leader_protocol_version,omitempty"`
	LeaderBinaryVersion   *string `json:"leader_binary_version,omitempty"`

	// Acp fields
	Payload string `json:"payload,omitempty"`

	// Error fields
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// ACP message types for the inner protocol.
type AcpMessage struct {
	MethodName string          `json:"method_name"`
	Request    json.RawMessage `json:"request"`
}

// NewSessionRequest is the request for session/new.
type NewSessionRequest struct {
	Cwd          string            `json:"cwd"`
	ModelID      *string           `json:"modelId,omitempty"`
	YoloMode     bool              `json:"yoloMode,omitempty"`
	AutoMode     bool              `json:"autoMode,omitempty"`
	ClientTerminal bool            `json:"clientTerminal,omitempty"`
	CodeNavEnabled bool            `json:"codeNavEnabled,omitempty"`
	Meta         map[string]interface{} `json:"_meta,omitempty"`
}

// NewSessionResponse is the response for session/new.
type NewSessionResponse struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

// LoadSessionRequest is the request for session/load.
type LoadSessionRequest struct {
	SessionID string                 `json:"session_id"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

// LoadSessionResponse is the response for session/load.
type LoadSessionResponse struct {
	SessionID string              `json:"session_id"`
	Cwd       string              `json:"cwd"`
	Messages  []AcpMessagePayload `json:"messages,omitempty"`
}

// PromptRequest is the request for session/prompt.
type PromptRequest struct {
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt"`
}

// PromptResponse is the response for session/prompt.
type PromptResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"` // "completed", "cancelled", "error"
	Message   string `json:"message,omitempty"`
}

// SessionUpdate is sent from server to client for streaming updates.
type SessionUpdate struct {
	SessionID string `json:"session_id"`
	EventID   uint64 `json:"event_id"`
	Type      string `json:"type"` // "agent_message_chunk", "tool_call", "tool_result", "turn_complete", "error"
	Data      json.RawMessage `json:"data"`
}

// AgentMessageChunk is a chunk of streaming agent response.
type AgentMessageChunk struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

// ToolCallUpdate is a tool call notification.
type ToolCallUpdate struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	Arguments string `json:"arguments"`
	Status    string `json:"status"` // "started", "completed", "failed"
	Result    string `json:"result,omitempty"`
}

// AcpMessagePayload is a generic ACP message payload for session history.
type AcpMessagePayload struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// InitializeResponse is the response for initialize.
type InitializeResponse struct {
	ProtocolVersion int    `json:"protocol_version"`
	ServerInfo      string `json:"server_info"`
	Capabilities    map[string]bool `json:"capabilities"`
}

// SetSessionModeRequest is the request for session/set_mode.
type SetSessionModeRequest struct {
	SessionID string `json:"session_id"`
	Mode      string `json:"mode"` // "plan" or "default"
}

// SetSessionModeResponse is the response for session/set_mode.
type SetSessionModeResponse struct {
	SessionID string `json:"session_id"`
	Mode      string `json:"mode"`
}

// CancelNotification is the request for session/cancel.
type CancelNotification struct {
	SessionID string `json:"session_id"`
}

// SetSessionModelRequest is the request for session/set_model.
type SetSessionModelRequest struct {
	SessionID string `json:"session_id"`
	ModelID   string `json:"model_id"`
}

// SetSessionModelResponse is the response for session/set_model.
type SetSessionModelResponse struct {
	SessionID string `json:"session_id"`
	ModelID   string `json:"model_id"`
}