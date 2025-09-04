package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nieveai/d-agents/internal/database"
	"github.com/nieveai/d-agents/internal/worker"
)

func main() {
	log.Println("Starting worker...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize the database connection
	db, err := database.NewSQLiteDatastore("d-agents.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize the worker
	if err := worker.Init(ctx, nil, db); err != nil {
		log.Fatalf("Failed to initialize worker: %v", err)
	}
	defer database.CloseNeo4jDriver()

	// In a real implementation, this worker would connect to the controller
	// to receive workloads. For now, it just starts and waits.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down worker...")
}
