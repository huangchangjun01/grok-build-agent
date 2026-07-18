package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	domain "github.com/spacexc/go-backend/internal/domain/tool"
	"github.com/spacexc/go-backend/internal/infrastructure/fs"
	"github.com/spacexc/go-backend/internal/infrastructure/shell"
	"github.com/spacexc/go-backend/internal/infrastructure/web"
)

// ─── Parameter helpers ────────────────────────────────────────────────────────

func getStringParam(params map[string]interface{}, key string) (string, bool) {
	val, ok := params[key]
	if !ok {
		return "", false
	}
	s, ok := val.(string)
	return s, ok
}

func getIntParam(params map[string]interface{}, key string) (int, bool) {
	val, ok := params[key]
	if !ok {
		return 0, false
	}
	switch v := val.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	}
	return 0, false
}

func getBoolParam(params map[string]interface{}, key string) (bool, bool) {
	val, ok := params[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// ─── Todo storage ─────────────────────────────────────────────────────────────

var (
	todoStore   = make(map[string][]map[string]interface{})
	todoStoreMu sync.RWMutex
)

// ─── ReadFileTool ─────────────────────────────────────────────────────────────

// ReadFileTool reads file content with optional pagination
type ReadFileTool struct {
	reader *fs.FileReader
	logger *logrus.Logger
}

// NewReadFileTool creates a new ReadFileTool
func NewReadFileTool(reader *fs.FileReader, logger *logrus.Logger) *ReadFileTool {
	return &ReadFileTool{reader: reader, logger: logger}
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Reads a file from the local filesystem. Supports pagination with offset and limit."
}

func (t *ReadFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path to the file to read",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "The line number to start reading from (1-based)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "The number of lines to read",
			},
		},
		"required": []string{"file_path"},
	}
}

func (t *ReadFileTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	filePath, ok := getStringParam(ctx.Params, "file_path")
	if !ok || filePath == "" {
		return domain.ToolResult{Success: false, Error: "file_path is required"}, nil
	}

	offset, _ := getIntParam(ctx.Params, "offset")
	limit, _ := getIntParam(ctx.Params, "limit")

	startLine := offset
	endLine := 0
	if limit > 0 && offset > 0 {
		endLine = offset + limit - 1
	}

	t.logger.WithFields(logrus.Fields{
		"path":   filePath,
		"offset": offset,
		"limit":  limit,
	}).Debug("read_file executing")

	result, err := t.reader.ReadFile(filePath, startLine, endLine, 0)
	if err != nil {
		t.logger.WithError(err).WithField("path", filePath).Error("read_file failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	output := result.Content
	if result.Truncated {
		output += "\n\n[Content truncated]"
	}

	return domain.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"path":        result.Path,
			"start_line":  result.StartLine,
			"end_line":    result.EndLine,
			"total_lines": result.TotalLines,
			"truncated":   result.Truncated,
		},
	}, nil
}

// ─── WriteFileTool ────────────────────────────────────────────────────────────

// WriteFileTool writes or creates a file
type WriteFileTool struct {
	writer *fs.FileWriter
	logger *logrus.Logger
}

// NewWriteFileTool creates a new WriteFileTool
func NewWriteFileTool(writer *fs.FileWriter, logger *logrus.Logger) *WriteFileTool {
	return &WriteFileTool{writer: writer, logger: logger}
}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Description() string {
	return "Writes a file to the local filesystem. Creates parent directories if they do not exist."
}

func (t *WriteFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write to the file",
			},
		},
		"required": []string{"file_path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	filePath, ok := getStringParam(ctx.Params, "file_path")
	if !ok || filePath == "" {
		return domain.ToolResult{Success: false, Error: "file_path is required"}, nil
	}

	content, ok := getStringParam(ctx.Params, "content")
	if !ok {
		return domain.ToolResult{Success: false, Error: "content is required"}, nil
	}

	t.logger.WithField("path", filePath).Debug("write_file executing")

	result, err := t.writer.WriteFile(filePath, content)
	if err != nil {
		t.logger.WithError(err).WithField("path", filePath).Error("write_file failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	return domain.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("File written successfully: %s (%d bytes)", filePath, result.Size),
		Data: map[string]interface{}{
			"path":        result.Path,
			"size":        result.Size,
			"created":     result.Created,
			"overwritten": result.Overwritten,
		},
	}, nil
}

// ─── SearchReplaceTool ────────────────────────────────────────────────────────

// SearchReplaceTool performs search and replace in a file
type SearchReplaceTool struct {
	replacer *fs.SearchReplacer
	logger   *logrus.Logger
}

// NewSearchReplaceTool creates a new SearchReplaceTool
func NewSearchReplaceTool(replacer *fs.SearchReplacer, logger *logrus.Logger) *SearchReplaceTool {
	return &SearchReplaceTool{replacer: replacer, logger: logger}
}

func (t *SearchReplaceTool) Name() string { return "search_replace" }

func (t *SearchReplaceTool) Description() string {
	return "Performs exact string replacements in a file. Replaces all occurrences of old_string with new_string."
}

func (t *SearchReplaceTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path to the file to modify",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace it with",
			},
		},
		"required": []string{"file_path", "old_string", "new_string"},
	}
}

func (t *SearchReplaceTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	filePath, ok := getStringParam(ctx.Params, "file_path")
	if !ok || filePath == "" {
		return domain.ToolResult{Success: false, Error: "file_path is required"}, nil
	}

	oldStr, ok := getStringParam(ctx.Params, "old_string")
	if !ok {
		return domain.ToolResult{Success: false, Error: "old_string is required"}, nil
	}

	newStr, ok := getStringParam(ctx.Params, "new_string")
	if !ok {
		return domain.ToolResult{Success: false, Error: "new_string is required"}, nil
	}

	t.logger.WithField("path", filePath).Debug("search_replace executing")

	result, err := t.replacer.Replace(filePath, oldStr, newStr)
	if err != nil {
		t.logger.WithError(err).WithField("path", filePath).Error("search_replace failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	output := fmt.Sprintf("Replaced %d occurrence(s) in %s", result.Replaced, filePath)
	if result.Diff != "" {
		output += "\n\n" + result.Diff
	}

	return domain.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"path":     result.Path,
			"replaced": result.Replaced,
		},
	}, nil
}

// ─── ListDirTool ──────────────────────────────────────────────────────────────

// ListDirTool lists directory contents
type ListDirTool struct {
	lister *fs.DirLister
	logger *logrus.Logger
}

// NewListDirTool creates a new ListDirTool
func NewListDirTool(lister *fs.DirLister, logger *logrus.Logger) *ListDirTool {
	return &ListDirTool{lister: lister, logger: logger}
}

func (t *ListDirTool) Name() string { return "list_dir" }

func (t *ListDirTool) Description() string {
	return "Lists the contents of a directory. Returns file names, sizes, modification times, and whether each entry is a directory."
}

func (t *ListDirTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path to the directory to list",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	dirPath, ok := getStringParam(ctx.Params, "path")
	if !ok || dirPath == "" {
		return domain.ToolResult{Success: false, Error: "path is required"}, nil
	}

	t.logger.WithField("path", dirPath).Debug("list_dir executing")

	result, err := t.lister.ListDir(dirPath)
	if err != nil {
		t.logger.WithError(err).WithField("path", dirPath).Error("list_dir failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Contents of %s:\n\n", dirPath))
	for _, f := range result.Files {
		entryType := "file"
		if f.IsDir {
			entryType = "dir "
		}
		sb.WriteString(fmt.Sprintf("%s  %10d  %s  %s\n",
			f.ModTime.Format("2006-01-02 15:04"),
			f.Size,
			entryType,
			f.Name,
		))
	}

	fileList := make([]map[string]interface{}, 0, len(result.Files))
	for _, f := range result.Files {
		fileList = append(fileList, map[string]interface{}{
			"name":     f.Name,
			"path":     f.Path,
			"size":     f.Size,
			"is_dir":   f.IsDir,
			"mod_time": f.ModTime.Format(time.RFC3339),
		})
	}

	return domain.ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"path":  result.Path,
			"files": fileList,
		},
	}, nil
}

// ─── GrepTool ─────────────────────────────────────────────────────────────────

// GrepTool performs regex search in files
type GrepTool struct {
	searcher *fs.GrepSearcher
	logger   *logrus.Logger
}

// NewGrepTool creates a new GrepTool
func NewGrepTool(searcher *fs.GrepSearcher, logger *logrus.Logger) *GrepTool {
	return &GrepTool{searcher: searcher, logger: logger}
}

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Description() string {
	return "Searches for a regex pattern in files under a directory. Supports file glob filtering and result limiting."
}

func (t *GrepTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The regex pattern to search for",
			},
			"dir": map[string]interface{}{
				"type":        "string",
				"description": "The directory to search in (defaults to current working directory)",
			},
			"include": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern to filter files (e.g. '*.go')",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return (default 1000)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	pattern, ok := getStringParam(ctx.Params, "pattern")
	if !ok || pattern == "" {
		return domain.ToolResult{Success: false, Error: "pattern is required"}, nil
	}

	dir, _ := getStringParam(ctx.Params, "dir")
	if dir == "" {
		dir = ctx.WorkDir
	}

	include, _ := getStringParam(ctx.Params, "include")
	maxResults, _ := getIntParam(ctx.Params, "max_results")

	t.logger.WithFields(logrus.Fields{
		"pattern": pattern,
		"dir":     dir,
		"include": include,
	}).Debug("grep executing")

	results, err := t.searcher.Search(pattern, dir, include, maxResults)
	if err != nil {
		t.logger.WithError(err).WithField("pattern", pattern).Error("grep failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	var sb strings.Builder
	if len(results) == 0 {
		sb.WriteString("No matches found.")
	} else {
		for _, r := range results {
			sb.WriteString(fmt.Sprintf("%s:%d: %s\n", r.File, r.Line, r.Content))
		}
	}

	matchList := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		matchList = append(matchList, map[string]interface{}{
			"file":    r.File,
			"line":    r.Line,
			"content": r.Content,
		})
	}

	return domain.ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"pattern":    pattern,
			"match_count": len(results),
			"matches":    matchList,
		},
	}, nil
}

// ─── GlobTool ─────────────────────────────────────────────────────────────────

// GlobTool finds files matching a glob pattern
type GlobTool struct {
	searcher *fs.GlobSearcher
	logger   *logrus.Logger
}

// NewGlobTool creates a new GlobTool
func NewGlobTool(searcher *fs.GlobSearcher, logger *logrus.Logger) *GlobTool {
	return &GlobTool{searcher: searcher, logger: logger}
}

func (t *GlobTool) Name() string { return "glob" }

func (t *GlobTool) Description() string {
	return "Finds files matching a glob pattern in a directory. Supports standard glob patterns like '*.go' or '**/*.go'."
}

func (t *GlobTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "The glob pattern to match files against",
			},
			"dir": map[string]interface{}{
				"type":        "string",
				"description": "The directory to search in (defaults to current working directory)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	pattern, ok := getStringParam(ctx.Params, "pattern")
	if !ok || pattern == "" {
		return domain.ToolResult{Success: false, Error: "pattern is required"}, nil
	}

	dir, _ := getStringParam(ctx.Params, "dir")
	if dir == "" {
		dir = ctx.WorkDir
	}

	t.logger.WithFields(logrus.Fields{
		"pattern": pattern,
		"dir":     dir,
	}).Debug("glob executing")

	matches, err := t.searcher.Search(pattern, dir)
	if err != nil {
		t.logger.WithError(err).WithField("pattern", pattern).Error("glob failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	var sb strings.Builder
	if len(matches) == 0 {
		sb.WriteString("No files matching the pattern.")
	} else {
		for _, m := range matches {
			sb.WriteString(m)
			sb.WriteByte('\n')
		}
	}

	return domain.ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"pattern":     pattern,
			"match_count": len(matches),
			"matches":     matches,
		},
	}, nil
}

// ─── BashTool ─────────────────────────────────────────────────────────────────

// BashTool executes shell commands
type BashTool struct {
	executor *shell.Executor
	logger   *logrus.Logger
}

// NewBashTool creates a new BashTool
func NewBashTool(executor *shell.Executor, logger *logrus.Logger) *BashTool {
	return &BashTool{executor: executor, logger: logger}
}

func (t *BashTool) Name() string { return "bash" }

func (t *BashTool) Description() string {
	return "Executes a shell command. Returns stdout, stderr, exit code, and duration. Commands time out after 120 seconds by default."
}

func (t *BashTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default 120)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	command, ok := getStringParam(ctx.Params, "command")
	if !ok || command == "" {
		return domain.ToolResult{Success: false, Error: "command is required"}, nil
	}

	timeoutSec, _ := getIntParam(ctx.Params, "timeout")

	cfg := shell.ExecConfig{
		WorkDir: ctx.WorkDir,
	}
	if timeoutSec > 0 {
		cfg.Timeout = time.Duration(timeoutSec) * time.Second
	}

	t.logger.WithFields(logrus.Fields{
		"command":  command,
		"work_dir": ctx.WorkDir,
		"timeout":  cfg.Timeout,
	}).Debug("bash executing")

	result, err := t.executor.Execute(context.Background(), command, cfg)
	if err != nil {
		t.logger.WithError(err).WithField("command", command).Error("bash failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	var sb strings.Builder
	if result.Stdout != "" {
		sb.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("STDERR:\n")
		sb.WriteString(result.Stderr)
	}
	if result.TimedOut {
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("[Command timed out]")
	}

	success := result.ExitCode == 0

	return domain.ToolResult{
		Success: success,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"command":   result.Command,
			"exit_code": result.ExitCode,
			"duration":  result.Duration.Seconds(),
			"timed_out": result.TimedOut,
		},
	}, nil
}

// ─── WebSearchTool ────────────────────────────────────────────────────────────

// WebSearchTool searches the web
type WebSearchTool struct {
	searchClient *web.SearchClient
	logger       *logrus.Logger
}

// NewWebSearchTool creates a new WebSearchTool
func NewWebSearchTool(searchClient *web.SearchClient, logger *logrus.Logger) *WebSearchTool {
	return &WebSearchTool{searchClient: searchClient, logger: logger}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Description() string {
	return "Searches the web using DuckDuckGo and returns the top results with titles, URLs, and snippets."
}

func (t *WebSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query",
			},
			"num": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return (default 10)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	query, ok := getStringParam(ctx.Params, "query")
	if !ok || query == "" {
		return domain.ToolResult{Success: false, Error: "query is required"}, nil
	}

	num, _ := getIntParam(ctx.Params, "num")

	t.logger.WithField("query", query).Debug("web_search executing")

	results, err := t.searchClient.Search(context.Background(), query, num)
	if err != nil {
		t.logger.WithError(err).WithField("query", query).Error("web_search failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		sb.WriteString(fmt.Sprintf("   %s\n\n", r.Snippet))
	}

	resultList := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		resultList = append(resultList, map[string]interface{}{
			"title":   r.Title,
			"url":     r.URL,
			"snippet": r.Snippet,
		})
	}

	return domain.ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"query":        query,
			"result_count": len(results),
			"results":      resultList,
		},
	}, nil
}

// ─── WebFetchTool ─────────────────────────────────────────────────────────────

// WebFetchTool fetches web page content
type WebFetchTool struct {
	fetchClient *web.FetchClient
	logger      *logrus.Logger
}

// NewWebFetchTool creates a new WebFetchTool
func NewWebFetchTool(fetchClient *web.FetchClient, logger *logrus.Logger) *WebFetchTool {
	return &WebFetchTool{fetchClient: fetchClient, logger: logger}
}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) Description() string {
	return "Fetches a web page and returns its text content. Strips HTML tags, scripts, and styles."
}

func (t *WebFetchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL of the page to fetch",
			},
			"max_size": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum content size in bytes to fetch (default 100KB)",
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	url, ok := getStringParam(ctx.Params, "url")
	if !ok || url == "" {
		return domain.ToolResult{Success: false, Error: "url is required"}, nil
	}

	maxSize, _ := getIntParam(ctx.Params, "max_size")

	t.logger.WithField("url", url).Debug("web_fetch executing")

	result, err := t.fetchClient.Fetch(context.Background(), url, maxSize)
	if err != nil {
		t.logger.WithError(err).WithField("url", url).Error("web_fetch failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	output := result.Content
	if result.Title != "" {
		output = "Title: " + result.Title + "\n\n" + output
	}

	return domain.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"url":          result.URL,
			"title":        result.Title,
			"status_code":  result.StatusCode,
			"content_type": result.ContentType,
			"size":         result.Size,
		},
	}, nil
}

// ─── TodoWriteTool ────────────────────────────────────────────────────────────

// TodoWriteTool manages todo lists
type TodoWriteTool struct {
	logger *logrus.Logger
}

// NewTodoWriteTool creates a new TodoWriteTool
func NewTodoWriteTool(logger *logrus.Logger) *TodoWriteTool {
	return &TodoWriteTool{logger: logger}
}

func (t *TodoWriteTool) Name() string { return "todo_write" }

func (t *TodoWriteTool) Description() string {
	return "Creates and manages a structured task list. Accepts an array of todo items with id, content, status, and priority."
}

func (t *TodoWriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"todos": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Unique identifier for the todo item",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The description of the todo item",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "The status of the todo item (pending, in_progress, completed)",
							"enum":        []string{"pending", "in_progress", "completed"},
						},
						"priority": map[string]interface{}{
							"type":        "string",
							"description": "The priority of the todo item (high, medium, low)",
							"enum":        []string{"high", "medium", "low"},
						},
					},
					"required": []string{"id", "content", "status", "priority"},
				},
				"description": "Array of todo items to store",
			},
			"merge": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to merge with existing todos (true) or replace (false)",
			},
		},
		"required": []string{"todos"},
	}
}

func (t *TodoWriteTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	rawTodos, ok := ctx.Params["todos"]
	if !ok {
		return domain.ToolResult{Success: false, Error: "todos is required"}, nil
	}

	todoBytes, err := json.Marshal(rawTodos)
	if err != nil {
		return domain.ToolResult{Success: false, Error: fmt.Sprintf("failed to parse todos: %v", err)}, nil
	}

	var todoItems []map[string]interface{}
	if err := json.Unmarshal(todoBytes, &todoItems); err != nil {
		return domain.ToolResult{Success: false, Error: fmt.Sprintf("failed to parse todos: %v", err)}, nil
	}

	merge, _ := getBoolParam(ctx.Params, "merge")

	todoStoreMu.Lock()
	defer todoStoreMu.Unlock()

	if merge {
		existing := todoStore[ctx.SessionID]
		for _, newItem := range todoItems {
			newID, _ := newItem["id"].(string)
			found := false
			for i, ex := range existing {
				exID, _ := ex["id"].(string)
				if exID == newID {
					existing[i] = newItem
					found = true
					break
				}
			}
			if !found {
				existing = append(existing, newItem)
			}
		}
		todoStore[ctx.SessionID] = existing
	} else {
		todoStore[ctx.SessionID] = todoItems
	}

	t.logger.WithFields(logrus.Fields{
		"session_id": ctx.SessionID,
		"count":      len(todoItems),
		"merge":      merge,
	}).Debug("todo_write executed")

	var sb strings.Builder
	sb.WriteString("Todo list updated:\n\n")
	for _, item := range todoStore[ctx.SessionID] {
		id, _ := item["id"].(string)
		content, _ := item["content"].(string)
		status, _ := item["status"].(string)
		priority, _ := item["priority"].(string)
		statusIcon := map[string]string{
			"pending":    "[ ]",
			"in_progress": "[~]",
			"completed":  "[x]",
		}
		icon := statusIcon[status]
		if icon == "" {
			icon = "[ ]"
		}
		sb.WriteString(fmt.Sprintf("%s [%s] [%s] %s: %s\n", icon, priority, status, id, content))
	}

	return domain.ToolResult{
		Success: true,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"todos": todoStore[ctx.SessionID],
		},
	}, nil
}

// ─── TaskTool ─────────────────────────────────────────────────────────────────

// TaskTool creates and manages background tasks
type TaskTool struct {
	executor *shell.Executor
	logger   *logrus.Logger
}

// NewTaskTool creates a new TaskTool
func NewTaskTool(executor *shell.Executor, logger *logrus.Logger) *TaskTool {
	return &TaskTool{executor: executor, logger: logger}
}

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Description() string {
	return "Creates and manages background tasks. Runs shell commands asynchronously and returns a task ID for tracking."
}

func (t *TaskTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to run in the background",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default 120)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *TaskTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	command, ok := getStringParam(ctx.Params, "command")
	if !ok || command == "" {
		return domain.ToolResult{Success: false, Error: "command is required"}, nil
	}

	timeoutSec, _ := getIntParam(ctx.Params, "timeout")

	cfg := shell.ExecConfig{
		WorkDir: ctx.WorkDir,
	}
	if timeoutSec > 0 {
		cfg.Timeout = time.Duration(timeoutSec) * time.Second
	}

	t.logger.WithFields(logrus.Fields{
		"command":  command,
		"work_dir": ctx.WorkDir,
	}).Debug("task creating background job")

	taskID, err := t.executor.ExecuteBackground(context.Background(), command, cfg)
	if err != nil {
		t.logger.WithError(err).WithField("command", command).Error("task failed to start")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	output := fmt.Sprintf("Background task started with ID: %s\nCommand: %s", taskID, command)

	return domain.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"task_id":  taskID,
			"command":  command,
			"work_dir": ctx.WorkDir,
		},
	}, nil
}

// ─── AskUserQuestionTool ──────────────────────────────────────────────────────

// AskUserQuestionTool asks the user a question
type AskUserQuestionTool struct {
	logger *logrus.Logger
}

// NewAskUserQuestionTool creates a new AskUserQuestionTool
func NewAskUserQuestionTool(logger *logrus.Logger) *AskUserQuestionTool {
	return &AskUserQuestionTool{logger: logger}
}

func (t *AskUserQuestionTool) Name() string { return "ask_user_question" }

func (t *AskUserQuestionTool) Description() string {
	return "Asks the user a question and returns their response. Use this when you need user input or clarification."
}

func (t *AskUserQuestionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"question": map[string]interface{}{
				"type":        "string",
				"description": "The question to ask the user",
			},
		},
		"required": []string{"question"},
	}
}

func (t *AskUserQuestionTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	question, ok := getStringParam(ctx.Params, "question")
	if !ok || question == "" {
		return domain.ToolResult{Success: false, Error: "question is required"}, nil
	}

	t.logger.WithField("question", question).Debug("ask_user_question executing")

	if ctx.UserApproval != nil {
		approved := ctx.UserApproval(question)
		if approved {
			return domain.ToolResult{
				Success: true,
				Output:  "User approved.",
			}, nil
		}
		return domain.ToolResult{
			Success: false,
			Output:  "User declined.",
		}, nil
	}

	return domain.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Question for user: %s\n[Awaiting user response]", question),
	}, nil
}

// ─── KillTaskTool ─────────────────────────────────────────────────────────────

// KillTaskTool cancels a running background task
type KillTaskTool struct {
	executor *shell.Executor
	logger   *logrus.Logger
}

// NewKillTaskTool creates a new KillTaskTool
func NewKillTaskTool(executor *shell.Executor, logger *logrus.Logger) *KillTaskTool {
	return &KillTaskTool{executor: executor, logger: logger}
}

func (t *KillTaskTool) Name() string { return "kill_task" }

func (t *KillTaskTool) Description() string {
	return "Cancels a running background task by its task ID."
}

func (t *KillTaskTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the background task to cancel",
			},
		},
		"required": []string{"task_id"},
	}
}

func (t *KillTaskTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	taskID, ok := getStringParam(ctx.Params, "task_id")
	if !ok || taskID == "" {
		return domain.ToolResult{Success: false, Error: "task_id is required"}, nil
	}

	t.logger.WithField("task_id", taskID).Debug("kill_task executing")

	if err := t.executor.CancelTask(taskID); err != nil {
		t.logger.WithError(err).WithField("task_id", taskID).Error("kill_task failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	return domain.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Task %s has been cancelled.", taskID),
		Data: map[string]interface{}{
			"task_id": taskID,
		},
	}, nil
}

// ─── TaskOutputTool ───────────────────────────────────────────────────────────

// TaskOutputTool retrieves the output of a background task
type TaskOutputTool struct {
	executor *shell.Executor
	logger   *logrus.Logger
}

// NewTaskOutputTool creates a new TaskOutputTool
func NewTaskOutputTool(executor *shell.Executor, logger *logrus.Logger) *TaskOutputTool {
	return &TaskOutputTool{executor: executor, logger: logger}
}

func (t *TaskOutputTool) Name() string { return "task_output" }

func (t *TaskOutputTool) Description() string {
	return "Gets the output of a running or completed background task by its task ID. Returns nil output if the task is still running."
}

func (t *TaskOutputTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the background task to get output for",
			},
		},
		"required": []string{"task_id"},
	}
}

func (t *TaskOutputTool) Execute(ctx domain.ToolContext) (domain.ToolResult, error) {
	taskID, ok := getStringParam(ctx.Params, "task_id")
	if !ok || taskID == "" {
		return domain.ToolResult{Success: false, Error: "task_id is required"}, nil
	}

	t.logger.WithField("task_id", taskID).Debug("task_output executing")

	result, err := t.executor.GetTaskOutput(taskID)
	if err != nil {
		t.logger.WithError(err).WithField("task_id", taskID).Error("task_output failed")
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	if result == nil {
		return domain.ToolResult{
			Success: true,
			Output:  fmt.Sprintf("Task %s is still running.", taskID),
			Data: map[string]interface{}{
				"task_id": taskID,
				"running": true,
			},
		}, nil
	}

	var sb strings.Builder
	if result.Stdout != "" {
		sb.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("STDERR:\n")
		sb.WriteString(result.Stderr)
	}
	if result.TimedOut {
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("[Task timed out]")
	}

	success := result.ExitCode == 0

	return domain.ToolResult{
		Success: success,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"task_id":   taskID,
			"command":   result.Command,
			"exit_code": result.ExitCode,
			"duration":  result.Duration.Seconds(),
			"timed_out": result.TimedOut,
			"running":   false,
		},
	}, nil
}