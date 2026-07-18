package shell

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"os/exec"
)

const (
	defaultTimeout   = 120 * time.Second
	defaultMaxOutput = 100 * 1024 // 100KB
)

// Executor executes shell commands safely
type Executor struct {
	logger *logrus.Logger
}

// ExecResult holds the result of a command execution
type ExecResult struct {
	Command  string        `json:"command"`
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	TimedOut bool          `json:"timed_out"`
}

// ExecConfig configures command execution
type ExecConfig struct {
	WorkDir   string        // working directory
	Timeout   time.Duration // max execution time (default 120s)
	Env       []string      // additional environment variables
	MaxOutput int           // max output bytes to capture (default 100KB)
}

type bgTask struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	result *ExecResult
	done   chan struct{}
}

// NewExecutor creates a new shell executor
func NewExecutor(logger *logrus.Logger) *Executor {
	return &Executor{logger: logger}
}

var (
	bgTasks   = make(map[string]*bgTask)
	bgTasksMu sync.Mutex
	taskIDSeq atomic.Int64
)

// Execute runs a command with the given configuration
func (e *Executor) Execute(ctx context.Context, command string, cfg ExecConfig) (*ExecResult, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxOutput <= 0 {
		cfg.MaxOutput = defaultMaxOutput
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = cfg.WorkDir
	if len(cfg.Env) > 0 {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{max: cfg.MaxOutput, buf: &stdout}
	cmd.Stderr = &limitWriter{max: cfg.MaxOutput, buf: &stderr}

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &ExecResult{
		Command:  command,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
		Duration: duration,
		TimedOut: false,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	e.logger.WithFields(logrus.Fields{
		"command":   command,
		"duration":  duration,
		"exit_code": result.ExitCode,
		"timed_out": result.TimedOut,
	}).Debug("shell command executed")

	return result, nil
}

// ExecuteBackground runs a command in the background, returns a task ID
func (e *Executor) ExecuteBackground(ctx context.Context, command string, cfg ExecConfig) (string, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxOutput <= 0 {
		cfg.MaxOutput = defaultMaxOutput
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = cfg.WorkDir
	if len(cfg.Env) > 0 {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{max: cfg.MaxOutput, buf: &stdout}
	cmd.Stderr = &limitWriter{max: cfg.MaxOutput, buf: &stderr}

	taskID := fmt.Sprintf("task-%d", taskIDSeq.Add(1))
	task := &bgTask{
		cmd:    cmd,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	bgTasksMu.Lock()
	bgTasks[taskID] = task
	bgTasksMu.Unlock()

	e.logger.WithFields(logrus.Fields{
		"task_id":  taskID,
		"command":  command,
		"work_dir": cfg.WorkDir,
	}).Debug("background task started")

	go func() {
		defer close(task.done)

		start := time.Now()
		err := cmd.Run()
		duration := time.Since(start)

		result := &ExecResult{
			Command:  command,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: 0,
			Duration: duration,
			TimedOut: false,
		}

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				result.TimedOut = true
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else {
				result.ExitCode = -1
			}
		}

		task.result = result

		e.logger.WithFields(logrus.Fields{
			"task_id":   taskID,
			"command":   command,
			"duration":  duration,
			"exit_code": result.ExitCode,
			"timed_out": result.TimedOut,
		}).Debug("background task completed")
	}()

	return taskID, nil
}

// GetTaskOutput retrieves the output of a background task
func (e *Executor) GetTaskOutput(taskID string) (*ExecResult, error) {
	bgTasksMu.Lock()
	task, ok := bgTasks[taskID]
	bgTasksMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	select {
	case <-task.done:
		return task.result, nil
	default:
		return nil, nil
	}
}

// CancelTask cancels a running background task
func (e *Executor) CancelTask(taskID string) error {
	bgTasksMu.Lock()
	task, ok := bgTasks[taskID]
	if !ok {
		bgTasksMu.Unlock()
		return fmt.Errorf("task %s not found", taskID)
	}
	delete(bgTasks, taskID)
	bgTasksMu.Unlock()

	task.cancel()

	e.logger.WithField("task_id", taskID).Debug("background task cancelled")
	return nil
}

// limitWriter wraps a bytes.Buffer and limits the total bytes written
type limitWriter struct {
	max int
	buf *bytes.Buffer
}

func (w *limitWriter) Write(p []byte) (int, error) {
	available := w.max - w.buf.Len()
	if available <= 0 {
		return len(p), nil
	}
	if len(p) > available {
		p = p[:available]
	}
	return w.buf.Write(p)
}