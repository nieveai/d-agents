package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/nieveai/d-agents/internal/database"
	"github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

type Config struct {
	Workers int `json:"workers"`
}

var agents = make(map[string]*models.Agent)

type Command func(workloadChan chan<- *pb.Workload, args []string)

var commands map[string]Command

func main() {
	// Command-line flags
	workers := flag.Int("workers", 0, "Number of workers")
	flag.Parse()

	// Configuration file
	config := &Config{}
	configFile, err := os.Open("config.json")
	if err == nil {
		defer configFile.Close()
		jsonParser := json.NewDecoder(configFile)
		jsonParser.Decode(config)
	}

	numWorkers := config.Workers
	if *workers > 0 {
		numWorkers = *workers
	}

	if numWorkers == 0 {
		numWorkers = 5 // Default value
	}

	log.Printf("Starting controller with %d workers", numWorkers)

	// Database
	db, err := database.NewSQLiteDatastore("d-agents.db")
	if err != nil {
		log.Fatalf("Error opening database: %s", err)
	}

	// Load agents from database
	dbAgents, err := db.ListAgents()
	if err != nil {
		log.Printf("Error loading agents from database: %s", err)
	}
	for _, agent := range dbAgents {
		agents[agent.ID] = agent
	}

	commands = map[string]Command{
		"/help": func(workloadChan chan<- *pb.Workload, args []string) {
			fmt.Println("Available commands:")
			fmt.Println("  /help - Show this help message")
			fmt.Println("  /list agent - List all registered agents")
			fmt.Println("  /add agent @<filename> - Add an agent from a configuration file")
			fmt.Println("  /session start <agent-id> - Create a new agent workload")
			fmt.Println("  /quit - Exit the program")
		},
		"/quit": func(workloadChan chan<- *pb.Workload, args []string) {
			os.Exit(0)
		},
		"/session": func(workloadChan chan<- *pb.Workload, args []string) {
			if len(args) > 1 && args[0] == "start" {
				agentID := args[1]
				agent, ok := agents[agentID]
				if !ok {
					fmt.Printf("Agent with ID '%s' not found.\n", agentID)
					return
				}

				workloadID := uuid.New().String()
				workload := &pb.Workload{
					Id:      workloadID,
					Name:    agent.Name,
					Type:    agent.ID,
					Payload: []byte{},
				}

				workloadChan <- workload
				fmt.Printf("Started session for agent %s with workload ID %s\n", agentID, workloadID)
			} else {
				fmt.Println("Usage: /session start <agent-id>")
			}
		},
		"/list": func(workloadChan chan<- *pb.Workload, args []string) {
			if len(args) > 0 && args[0] == "agent" {
				if len(agents) == 0 {
					fmt.Println("No agents registered.")
					return
				}
				for _, agent := range agents {
                    fmt.Printf("  - %s: %s\n    Description: %s\n", agent.ID, agent.Name, agent.Description)
                }
			} else {
				fmt.Println("Unknown subcommand for /list. Try '/list agent'")
			}
		},
		"/add": func(workloadChan chan<- *pb.Workload, args []string) {
			if len(args) > 0 && args[0] == "agent" {
				if len(args) > 1 && strings.HasPrefix(args[1], "@") {
					filename := strings.TrimPrefix(args[1], "@")
					file, err := os.Open(filename)
					if err != nil {
						fmt.Printf("Error opening file: %s\n", err)
						return
					}
					defer file.Close()

					var agent models.Agent
					decoder := json.NewDecoder(file)
					if err := decoder.Decode(&agent); err != nil {
						fmt.Printf("Error decoding agent file: %s\n", err)
						return
					}

					if err := db.AddAgent(&agent); err != nil {
						fmt.Printf("Error adding agent to database: %s\n", err)
						return
					}

					agents[agent.ID] = &agent
					fmt.Printf("Agent '%s' with ID '%s' added.\n", agent.Name, agent.ID)
				} else {
					fmt.Println("Usage: /add agent @<filename>")
				}
			} else {
				fmt.Println("Unknown subcommand for /add. Try '/add agent'")
			}
		},
	}

	workloadChan := make(chan *pb.Workload)

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		go worker(i, workloadChan)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Enter workload command (e.g., 'echo hello world') or /help for commands:")

	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			if cmd, ok := commands[parts[0]]; ok {
				cmd(workloadChan, parts[1:])
			} else {
				fmt.Println("Unknown command. Type /help for a list of commands.")
			}
			continue
		}

		parts := strings.SplitN(input, " ", 2)

		if len(parts) < 2 {
			if parts[0] == "exit" {
				close(workloadChan)
				return
			}
			fmt.Println("Invalid command. Please use the format 'type payload'")
			continue
		}

		workload := &pb.Workload{
			Id:      "workload-1", // This should be generated
			Name:    "cli-workload",
			Type:    parts[0],
			Payload: []byte(parts[1]),
		}

		workloadChan <- workload
	}
}

func worker(id int, workloadChan <-chan *pb.Workload) {
	for workload := range workloadChan {
		log.Printf("Worker %d processing workload: %s", id, workload.Type)
		// Simulate work
		fmt.Printf("Worker %d: Echoing payload: %s\n", id, string(workload.Payload))
	}
	log.Printf("Worker %d shutting down", id)
}
