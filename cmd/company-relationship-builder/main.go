package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/nieveai/d-agents/internal/agents"
	"github.com/nieveai/d-agents/internal/database"
	"github.com/nieveai/d-agents/internal/models"
	"github.com/nieveai/d-agents/internal/worker"
	pb "github.com/nieveai/d-agents/proto"
)

func main() {
	// --- Command-line Flags ---
	modelID := flag.String("model", "", "The ID of the model to use for processing. This flag is required.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -model <model_id> <file_path>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Processes a list of company names from a text file to find and store their relationships.\n\n")
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  <file_path>\n\tThe path to a text file containing company names, one per line.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *modelID == "" || flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}
	filePath := flag.Arg(0)
	// --- End Flags ---

	// --- Database and Model Initialization ---
	db, err := database.NewSQLiteDatastore("d-agents.db")
	if err != nil {
		log.Fatalf("Error opening database: %s", err)
	}

	dbModels, err := db.ListModels()
	if err != nil {
		log.Fatalf("Error loading models from database: %s", err)
	}

	if len(dbModels) == 0 {
		log.Fatal("No models found in the database. Please add a model using the controller program first.")
	}

	var selectedModel *models.Model
	for _, m := range dbModels {
		if m.ID == *modelID {
			selectedModel = m
			break
		}
	}

	if selectedModel == nil {
		log.Fatalf("Model with ID '%s' not found in the database.", *modelID)
	}

	log.Printf("Using model: %s (%s/%s)", selectedModel.ID, selectedModel.Provider, selectedModel.ModelID)

	genAIClient, err := worker.NewLLMClient(context.Background(), dbModels)
	if err != nil {
		log.Fatalf("Failed to create GenAI client: %v", err)
	}
	// --- End Initialization ---

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	companyAgent, err := agents.NewCompanyRelationshipAgent()
	if err != nil {
		log.Fatalf("Failed to create company relationship agent: %v", err)
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		companyName := scanner.Text()
		if companyName == "" {
			continue
		}

		fmt.Printf("Processing company: %s\n", companyName)

		workload := &pb.Workload{
			Id:      uuid.New().String(),
			Name:    companyName,
			Payload: []byte(fmt.Sprintf("find the relationship for %s", companyName)),
			Models:  []string{selectedModel.ID},
			Status:  pb.WorkloadStatus_RUNNING,
		}

		if err := companyAgent.DoWork(workload, genAIClient); err != nil {
			log.Printf("Failed to process workload for %s: %v", companyName, err)
		} else {
			fmt.Printf("Successfully processed and stored relationships for %s\n", companyName)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}
}