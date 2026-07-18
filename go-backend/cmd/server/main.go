package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/spacexc/go-backend/internal/infrastructure/persistence"
	"github.com/spacexc/go-backend/internal/interfaces/http"
	"github.com/spacexc/go-backend/pkg/config"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := config.InitLogger(cfg)
	log.Info("Starting SpaceXC Grok Build Go Backend...")
	log.Debugf("Config loaded: server=%s:%d, llm_provider=%s, llm_model=%s",
		cfg.Server.Host, cfg.Server.Port, cfg.LLM.Provider, cfg.LLM.DefaultModel)

	// Ensure data directories exist
	if err := ensureDataDirs(cfg, log); err != nil {
		log.Fatalf("Failed to create data directories: %v", err)
	}

	// Initialize database
	db, err := persistence.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Info("Database initialized successfully")

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// Register API routes
	handler := http.NewHandler(cfg, db, log)
	handler.RegisterRoutes(router)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Infof("Server starting on %s", addr)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := router.Run(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-quit
	log.Info("Shutting down server...")
	log.Info("Server stopped")
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