package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"
)

// MCPClient implements the Model Context Protocol client.
type MCPClient struct {
	servers map[string]*MCPServer
	logger  *logrus.Logger
	mu      sync.RWMutex
}

// MCPServer represents a managed MCP server.
type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Status  string            `json:"status"` // "stopped", "starting", "running", "error"
	Tools   []MCPTool         `json:"tools"`
	cmd     *exec.Cmd
	cancel  context.CancelFunc
}

// MCPTool represents a tool provided by an MCP server.
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	ServerName  string                 `json:"server_name"`
}

// NewMCPClient creates a new MCPClient.
func NewMCPClient(logger *logrus.Logger) *MCPClient {
	return &MCPClient{
		servers: make(map[string]*MCPServer),
		logger:  logger,
	}
}

// StartServer starts an MCP server.
func (c *MCPClient) StartServer(ctx context.Context, name, command string, args []string, env map[string]string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.servers[name]; exists {
		return fmt.Errorf("server %s already exists", name)
	}

	serverCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(serverCtx, command, args...)
	cmd.Env = buildEnv(env)

	server := &MCPServer{
		Name:    name,
		Command: command,
		Args:    args,
		Env:     env,
		Status:  "starting",
		Tools:   make([]MCPTool, 0),
		cmd:     cmd,
		cancel:  cancel,
	}

	c.servers[name] = server

	c.logger.WithFields(logrus.Fields{
		"server_name": name,
		"command":     command,
		"args":        args,
	}).Info("starting MCP server")

	// Start the server process
	if err := cmd.Start(); err != nil {
		server.Status = "error"
		c.logger.WithFields(logrus.Fields{
			"server_name": name,
			"error":       err,
		}).Error("failed to start MCP server")
		return fmt.Errorf("failed to start MCP server %s: %w", name, err)
	}

	server.Status = "running"

	// Monitor the process in the background
	go c.monitorServer(name, cmd)

	c.logger.WithFields(logrus.Fields{
		"server_name": name,
		"pid":         cmd.Process.Pid,
	}).Info("MCP server started")

	return nil
}

// StopServer stops an MCP server.
func (c *MCPClient) StopServer(name string) error {
	c.mu.Lock()
	server, exists := c.servers[name]
	if !exists {
		c.mu.Unlock()
		return fmt.Errorf("server %s not found", name)
	}

	c.logger.WithFields(logrus.Fields{
		"server_name": name,
	}).Info("stopping MCP server")

	if server.cmd != nil && server.cmd.Process != nil {
		// Send SIGTERM to the process
		if err := server.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			c.logger.WithFields(logrus.Fields{
				"server_name": name,
				"error":       err,
			}).Warn("failed to send SIGTERM, sending SIGKILL")

			if killErr := server.cmd.Process.Kill(); killErr != nil {
				c.logger.WithFields(logrus.Fields{
					"server_name": name,
					"error":       killErr,
				}).Error("failed to kill MCP server process")
			}
		}
	}

	if server.cancel != nil {
		server.cancel()
	}

	server.Status = "stopped"
	c.mu.Unlock()

	// Wait for the process to exit
	if server.cmd != nil {
		_ = server.cmd.Wait()
	}

	c.logger.WithFields(logrus.Fields{
		"server_name": name,
	}).Info("MCP server stopped")

	return nil
}

// ListServers lists all managed MCP servers.
func (c *MCPClient) ListServers() []*MCPServer {
	c.mu.RLock()
	defer c.mu.RUnlock()

	servers := make([]*MCPServer, 0, len(c.servers))
	for _, s := range c.servers {
		servers = append(servers, s)
	}
	return servers
}

// GetServerStatus returns the status of an MCP server.
func (c *MCPClient) GetServerStatus(name string) (*MCPServer, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	server, exists := c.servers[name]
	if !exists {
		return nil, fmt.Errorf("server %s not found", name)
	}

	return server, nil
}

// DiscoverTools discovers tools from an MCP server.
// This is a simplified implementation that performs a basic handshake
// via JSON-RPC over stdio.
func (c *MCPClient) DiscoverTools(ctx context.Context, name string) ([]MCPTool, error) {
	c.mu.RLock()
	server, exists := c.servers[name]
	if !exists {
		c.mu.RUnlock()
		return nil, fmt.Errorf("server %s not found", name)
	}
	c.mu.RUnlock()

	if server.Status != "running" {
		return nil, fmt.Errorf("server %s is not running (status: %s)", name, server.Status)
	}

	c.logger.WithFields(logrus.Fields{
		"server_name": name,
	}).Info("discovering tools from MCP server")

	// Simplified JSON-RPC handshake: send initialize request
	initializeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "0.1.0",
			"clientInfo": map[string]string{
				"name":    "go-backend",
				"version": "1.0.0",
			},
		},
	}

	reqBytes, err := json.Marshal(initializeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal initialize request: %w", err)
	}

	// For now, return a basic handshake result
	// In a full implementation, this would communicate over the server's stdio
	c.logger.WithFields(logrus.Fields{
		"server_name": name,
		"request":     string(reqBytes),
	}).Debug("MCP initialize request prepared")

	// Return any existing tools discovered for this server
	tools := make([]MCPTool, 0, len(server.Tools))
	for _, t := range server.Tools {
		tools = append(tools, t)
	}

	return tools, nil
}

// ListAllTools lists all tools from all MCP servers.
func (c *MCPClient) ListAllTools() []MCPTool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tools := make([]MCPTool, 0)
	for _, server := range c.servers {
		for _, t := range server.Tools {
			tools = append(tools, t)
		}
	}
	return tools
}

// HealthCheck performs a liveness check on all servers.
func (c *MCPClient) HealthCheck(ctx context.Context) {
	c.mu.RLock()
	servers := make([]*MCPServer, 0, len(c.servers))
	for _, s := range c.servers {
		servers = append(servers, s)
	}
	c.mu.RUnlock()

	for _, server := range servers {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if server.Status != "running" {
			continue
		}

		if server.cmd == nil || server.cmd.Process == nil {
			c.mu.Lock()
			server.Status = "error"
			c.mu.Unlock()
			c.logger.WithFields(logrus.Fields{
				"server_name": server.Name,
			}).Warn("MCP server process is nil, marking as error")
			continue
		}

		// Check if the process is still running
		if err := server.cmd.Process.Signal(syscall.Signal(0)); err != nil {
			c.mu.Lock()
			server.Status = "error"
			c.mu.Unlock()
			c.logger.WithFields(logrus.Fields{
				"server_name": server.Name,
				"error":       err,
			}).Warn("MCP server health check failed")
		} else {
			c.logger.WithFields(logrus.Fields{
				"server_name": server.Name,
			}).Debug("MCP server health check passed")
		}
	}
}

// HandleCredentials stores credentials for MCP servers.
func (c *MCPClient) HandleCredentials(serverName string, credentials map[string]string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	server, exists := c.servers[serverName]
	if !exists {
		return errors.New("server not found")
	}

	// Merge credentials into the server's environment
	if server.Env == nil {
		server.Env = make(map[string]string)
	}
	for key, value := range credentials {
		server.Env[key] = value
	}

	c.logger.WithFields(logrus.Fields{
		"server_name":      serverName,
		"credential_count": len(credentials),
	}).Info("credentials stored for MCP server")

	return nil
}

// monitorServer monitors the MCP server process and updates its status.
func (c *MCPClient) monitorServer(name string, cmd *exec.Cmd) {
	err := cmd.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	server, exists := c.servers[name]
	if !exists {
		return
	}

	if err != nil {
		server.Status = "error"
		c.logger.WithFields(logrus.Fields{
			"server_name": name,
			"error":       err,
		}).Error("MCP server process exited with error")
	} else {
		if server.Status != "stopped" {
			server.Status = "stopped"
			c.logger.WithFields(logrus.Fields{
				"server_name": name,
			}).Info("MCP server process exited normally")
		}
	}
}

// buildEnv builds the environment variables for the MCP server process.
func buildEnv(env map[string]string) []string {
	// Start with the current process environment
	envVars := make([]string, 0, len(env))
	for key, value := range env {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}
	return envVars
}

// SetServerStatus sets the status of an MCP server (used internally and for testing).
func (c *MCPClient) SetServerStatus(name string, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if server, exists := c.servers[name]; exists {
		server.Status = status
		c.logger.WithFields(logrus.Fields{
			"server_name": name,
			"status":      status,
		}).Debug("MCP server status updated")
	}
}

// AddTool adds a tool to an MCP server's tool list.
func (c *MCPClient) AddTool(serverName string, tool MCPTool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if server, exists := c.servers[serverName]; exists {
		tool.ServerName = serverName
		server.Tools = append(server.Tools, tool)
		c.logger.WithFields(logrus.Fields{
			"server_name": serverName,
			"tool_name":   tool.Name,
		}).Debug("MCP tool added")
	}
}

// StopAll stops all managed MCP servers.
func (c *MCPClient) StopAll() {
	c.mu.RLock()
	names := make([]string, 0, len(c.servers))
	for name := range c.servers {
		names = append(names, name)
	}
	c.mu.RUnlock()

	for _, name := range names {
		if err := c.StopServer(name); err != nil {
			c.logger.WithFields(logrus.Fields{
				"server_name": name,
				"error":       err,
			}).Warn("error stopping MCP server during shutdown")
		}
	}
}

// ServerCount returns the number of managed MCP servers.
func (c *MCPClient) ServerCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.servers)
}