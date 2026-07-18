package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sirupsen/logrus"

	"github.com/spacexc/grok-build/internal/application/agent"
	"github.com/spacexc/grok-build/internal/application/session"
	apptool "github.com/spacexc/grok-build/internal/application/tool"
	domaintool "github.com/spacexc/grok-build/internal/domain/tool"
	"github.com/spacexc/grok-build/internal/infrastructure/fs"
	"github.com/spacexc/grok-build/internal/infrastructure/git"
	"github.com/spacexc/grok-build/internal/infrastructure/llm"
	"github.com/spacexc/grok-build/internal/infrastructure/persistence"
	"github.com/spacexc/grok-build/internal/infrastructure/shell"
	"github.com/spacexc/grok-build/internal/infrastructure/web"
	"github.com/spacexc/grok-build/internal/interfaces/tui"
	"github.com/spacexc/grok-build/pkg/config"
)

func main() {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize logger
	log := config.InitLogger(cfg)
	log.Info("Starting SpaceXC Grok Build TUI...")

	// 3. Ensure data directories exist
	if err := ensureDataDirs(cfg, log); err != nil {
		log.Fatalf("Failed to create data directories: %v", err)
	}

	// 4. Initialize database
	db, err := persistence.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Info("Database initialized successfully")

	// 5. Create infrastructure
	llmClient := llm.NewClient(&cfg.LLM, log)
	sessionRepo := persistence.NewSessionRepo(db, cfg.Storage.JSONLDir, cfg.Storage.SessionsDir, log)
	fileReader := fs.NewFileReader(log)
	fileWriter := fs.NewFileWriter(log)
	dirLister := fs.NewDirLister(log)
	grepSearcher := fs.NewGrepSearcher(log)
	globSearcher := fs.NewGlobSearcher(log)
	searchReplacer := fs.NewSearchReplacer(log)
	shellExecutor := shell.NewExecutor(log)
	webSearchClient := web.NewSearchClient(log)
	webFetchClient := web.NewFetchClient(log)
	gitClient := git.NewClient(log)

	// 6. Create application services
	toolRegistry := apptool.NewToolRegistry(log)
	sessionSvc := session.NewService(sessionRepo, llmClient, cfg, log)
	goalManager := agent.NewGoalManager(log)
	planModeManager := agent.NewPlanModeManager(log)

	// 7. Create agent
	ag := agent.NewAgent(cfg, llmClient, toolRegistry, sessionRepo, log)

	// 8. Register all tools
	registerAllTools(
		toolRegistry, fileReader, fileWriter, dirLister, grepSearcher,
		globSearcher, searchReplacer, shellExecutor, webSearchClient,
		webFetchClient, gitClient, goalManager, planModeManager, log,
	)

	// 9. Determine work directory
	workDir := cfg.Workspace.DefaultWorkDir
	if workDir == "" {
		workDir = "."
	}

	// 10. Create TUI model and run
	model := tui.NewModel(cfg, log, ag, sessionSvc, workDir)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		log.Fatalf("TUI failed: %v", err)
	}

	log.Info("TUI exited successfully")
}

func ensureDataDirs(cfg *config.Config, log *logrus.Logger) error {
	dirs := []string{
		"./data",
		"./data/sessions",
		"./data/jsonl",
		"./data/logs",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Errorf("Failed to create directory %s: %v", dir, err)
			return err
		}
	}
	return nil
}

// registerAllTools registers all tools into the registry.
func registerAllTools(
	registry *apptool.ToolRegistry,
	fileReader *fs.FileReader,
	fileWriter *fs.FileWriter,
	dirLister *fs.DirLister,
	grepSearcher *fs.GrepSearcher,
	globSearcher *fs.GlobSearcher,
	searchReplacer *fs.SearchReplacer,
	shellExecutor *shell.Executor,
	webSearchClient *web.SearchClient,
	webFetchClient *web.FetchClient,
	gitClient *git.Client,
	goalManager *agent.GoalManager,
	planModeManager *agent.PlanModeManager,
	logger *logrus.Logger,
) {
	// Core tools
	registry.Register(apptool.NewReadFileTool(fileReader, logger))
	registry.Register(apptool.NewWriteFileTool(fileWriter, logger))
	registry.Register(apptool.NewSearchReplaceTool(searchReplacer, logger))
	registry.Register(apptool.NewListDirTool(dirLister, logger))
	registry.Register(apptool.NewGrepTool(grepSearcher, logger))
	registry.Register(apptool.NewGlobTool(globSearcher, logger))
	registry.Register(apptool.NewBashTool(shellExecutor, logger))
	registry.Register(apptool.NewWebSearchTool(webSearchClient, logger))
	registry.Register(apptool.NewWebFetchTool(webFetchClient, logger))
	registry.Register(apptool.NewTodoWriteTool(logger))
	registry.Register(apptool.NewTaskTool(shellExecutor, logger))
	registry.Register(apptool.NewAskUserQuestionTool(logger))
	registry.Register(apptool.NewKillTaskTool(shellExecutor, logger))
	registry.Register(apptool.NewTaskOutputTool(shellExecutor, logger))

	// Advanced tools
	registry.Register(apptool.NewEnterPlanModeTool(logger, func(sessionID string, enabled bool) error {
		if enabled {
			return planModeManager.EnterPlanMode(sessionID)
		}
		return planModeManager.ExitPlanMode(sessionID)
	}))
	registry.Register(apptool.NewExitPlanModeTool(logger, func(sessionID string, enabled bool) error {
		if enabled {
			return planModeManager.EnterPlanMode(sessionID)
		}
		return planModeManager.ExitPlanMode(sessionID)
	}))
	registry.Register(apptool.NewUpdateGoalTool(logger, func(sessionID string, goal string) error {
		goalManager.CreateGoal(sessionID, goal)
		return nil
	}))
	registry.Register(apptool.NewSchedulerTool(shellExecutor, logger))
	registry.Register(apptool.NewImageGenTool(logger))
	registry.Register(apptool.NewImageEditTool(logger))
	registry.Register(apptool.NewVideoGenTool(logger))
	registry.Register(apptool.NewMermaidTool(logger))
	registry.Register(apptool.NewMemoryTool(logger))
	registry.Register(apptool.NewLSPTool(logger))

	// Git tool
	registry.Register(newGitTool(gitClient, logger))
}

// --- Git Tool Wrapper ---

type gitTool struct {
	client *git.Client
	logger *logrus.Logger
}

func newGitTool(client *git.Client, logger *logrus.Logger) *gitTool {
	return &gitTool{client: client, logger: logger}
}

func (t *gitTool) Name() string { return "git" }

func (t *gitTool) Description() string {
	return "Provides git repository operations: status, diff, and tracked files listing. Use this to check the state of a git repository."
}

func (t *gitTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The git action to perform: 'status', 'diff', or 'tracked_files'.",
				"enum":        []string{"status", "diff", "tracked_files"},
			},
			"repo_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the git repository. Defaults to current working directory.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *gitTool) Execute(ctx domaintool.ToolContext) (domaintool.ToolResult, error) {
	action, _ := ctx.Params["action"].(string)
	repoPath, ok := ctx.Params["repo_path"].(string)
	if !ok || repoPath == "" {
		repoPath = ctx.WorkDir
	}

	switch action {
	case "status":
		status, err := t.client.GetStatus(repoPath)
		if err != nil {
			return domaintool.ToolResult{Success: false, Error: err.Error()}, nil
		}
		output := fmt.Sprintf("Branch: %s\nClean: %v\nModified: %v\nAdded: %v\nDeleted: %v\nUntracked: %v",
			status.Branch, status.IsClean, status.Modified, status.Added, status.Deleted, status.Untracked)
		return domaintool.ToolResult{
			Success: true,
			Output:  output,
			Data: map[string]interface{}{
				"branch":    status.Branch,
				"is_clean":  status.IsClean,
				"modified":  status.Modified,
				"added":     status.Added,
				"deleted":   status.Deleted,
				"untracked": status.Untracked,
			},
		}, nil

	case "diff":
		staged := false
		if s, ok := ctx.Params["staged"].(bool); ok {
			staged = s
		}
		diff, err := t.client.GetDiff(repoPath, staged)
		if err != nil {
			return domaintool.ToolResult{Success: false, Error: err.Error()}, nil
		}
		return domaintool.ToolResult{
			Success: true,
			Output:  diff.Diff,
			Data: map[string]interface{}{
				"staged": diff.Staged,
				"diff":   diff.Diff,
			},
		}, nil

	case "tracked_files":
		files, err := t.client.GetTrackedFiles(repoPath)
		if err != nil {
			return domaintool.ToolResult{Success: false, Error: err.Error()}, nil
		}
		output := fmt.Sprintf("Tracked files (%d):\n", len(files))
		for _, f := range files {
			output += f + "\n"
		}
		return domaintool.ToolResult{
			Success: true,
			Output:  output,
			Data: map[string]interface{}{
				"files": files,
				"count": len(files),
			},
		}, nil

	default:
		return domaintool.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("unknown git action: %s (supported: status, diff, tracked_files)", action),
		}, nil
	}
}