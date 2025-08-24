package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/nieveai/d-agents/internal/database"
	"github.com/nieveai/d-agents/internal/models"
	"github.com/nieveai/d-agents/internal/worker"
	pb "github.com/nieveai/d-agents/proto"
)

type Config struct {
	Workers       int    `json:"workers"`
	GeminiAPIKey string `json:"gemini_api_key"`
}

var agents = make(map[string]*models.Agent)
var sessions = make(map[string]*pb.Workload)
var currentSession *pb.Workload
var inPayloadInputMode = false
var payloadBuffer strings.Builder

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

	if config.GeminiAPIKey != "" {
		if err := worker.Init(context.Background(), config.GeminiAPIKey); err != nil {
			log.Fatalf("Error initializing worker: %s", err)
		}
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
			fmt.Println("  /list session - List all created sessions")
			fmt.Println("  /add agent @<filename> - Add an agent from a configuration file")
			fmt.Println("  /session start <agent-id> - Create a new agent workload")
			fmt.Println("  /session run [session-id] - Run the current session or a specific session by ID")
			fmt.Println("  /session save - Save the current session")
			fmt.Println("  /quit - Exit the program")
		},
		"/quit": func(workloadChan chan<- *pb.Workload, args []string) {
			os.Exit(0)
		},
		"/session": func(workloadChan chan<- *pb.Workload, args []string) {
			if len(args) > 0 {
				switch args[0] {
				case "start":
					if len(args) > 1 {
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
						}

						sessions[workloadID] = workload
						currentSession = workload
						inPayloadInputMode = true
						payloadBuffer.Reset()
						fmt.Println("what would you like the agent to do? Please enter your instruction below.")
					} else {
						fmt.Println("Usage: /session start <agent-id>")
					}

				case "run":
					if len(args) > 1 {
						sessionID := args[1]
						session, ok := sessions[sessionID]
						if !ok {
							fmt.Printf("Session with ID '%s' not found.\n", sessionID)
							return
						}
						workloadChan <- session
						fmt.Printf("Running session with workload ID %s\n", session.Id)
					} else {
						if currentSession != nil {
							inPayloadInputMode = false
							currentSession.Payload = []byte(payloadBuffer.String())
							workloadChan <- currentSession
							fmt.Printf("Running session with workload ID %s\n", currentSession.Id)
							payloadBuffer.Reset()
						} else {
							fmt.Println("No active session. Use '/session start <agent-id>' to start one.")
						}
					}

				case "save":
					if currentSession != nil {
						inPayloadInputMode = false
						currentSession.Payload = []byte(payloadBuffer.String())
						sessions[currentSession.Id] = currentSession
						fmt.Printf("Saved session with workload ID %s\n", currentSession.Id)
						payloadBuffer.Reset()
					} else {
						fmt.Println("No active session. Use '/session start <agent-id>' to start one.")
					}
				default:
					fmt.Println("Unknown command for /session. Available commands: start, run, save")
				}
			} else {
				fmt.Println("Usage: /session <start|run|save>")
			}
		},
		"/list": func(workloadChan chan<- *pb.Workload, args []string) {
			if len(args) > 0 {
				switch args[0] {
				case "agent":
					if len(agents) == 0 {
						fmt.Println("No agents registered.")
						return
					}
					for _, agent := range agents {
						fmt.Printf("  - %s: %s\n    Description: %s\n", agent.ID, agent.Name, agent.Description)
					}
				case "session":
					if len(sessions) == 0 {
						fmt.Println("No sessions created.")
						return
					}

					for _, session := range sessions {
						payload := string(session.Payload)
						if len(payload) > 50 {
							payload = payload[:50] + "..."
						}
						fmt.Printf("  - %s: %s\n    Payload: %s\n", session.Id, session.Name, payload)
					}

				default:
					fmt.Println("Unknown subcommand for /list. Try '/list agent' or '/list session'")
				}
			} else {
				fmt.Println("Usage: /list <agent|session>")
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
		go runWorker(i, workloadChan)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Enter workload command (e.g., 'echo hello world') or /help for commands:")

	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if inPayloadInputMode {
			if strings.HasPrefix(input, "/") {
				parts := strings.Fields(input)
				if cmd, ok := commands[parts[0]]; ok {
					cmd(workloadChan, parts[1:])
				} else {
					fmt.Println("Unknown command. Type /help for a list of commands.")
				}
			} else {
				payloadBuffer.WriteString(input)
				payloadBuffer.WriteString("\n")
			}
			continue
		}

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

	}
}

func runWorker(id int, workloadChan <-chan *pb.Workload) {
	for workload := range workloadChan {
		log.Printf("Worker %d processing workload: %s", id, workload.Type)
		worker.ProcessWorkload(workload)
	}
	log.Printf("Worker %d shutting down", id)
}
