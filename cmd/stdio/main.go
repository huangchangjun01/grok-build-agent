package main

import (
	"fmt"
	"os"

	"github.com/spacexc/grok-build/internal/infrastructure/persistence"
	"github.com/spacexc/grok-build/internal/interfaces/stdio"
	"github.com/spacexc/grok-build/pkg/config"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger (stderr to avoid interfering with stdio protocol)
	logger := config.InitLogger(cfg)
	logger.SetOutput(os.Stderr) // Log to stderr, stdout is for protocol

	logger.Info("Starting SpaceXC Grok Build Go Backend (Stdio mode)...")

	// Ensure data directories exist
	dirs := []string{"./data", "./data/sessions", "./data/jsonl", "./data/logs"}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Initialize database
	db, err := persistence.InitDB(cfg)
	if err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	logger.Info("Database initialized successfully")

	// Create and run stdio server
	server := stdio.NewServer(cfg, db, logger)
	if err := server.Run(); err != nil {
		logger.Fatalf("Server error: %v", err)
	}

	logger.Info("Server stopped")
}