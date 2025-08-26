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
	Workers int `json:"workers"`
}

var agents = make(map[string]*models.Agent)
var modelStore = make(map[string]*models.Model)
var sessions = make(map[string]*pb.Workload)
var currentSession *pb.Workload
var inPayloadInputMode = false
var payloadBuffer strings.Builder

type Command func(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, args []string)

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

	// Load sessions from database
	dbSessions, err := db.ListSessions()
	if err != nil {
		log.Printf("Error loading sessions from database: %s", err)
	}
	for _, session := range dbSessions {
		sessions[session.Id] = session
	}

	// Load models from database
	dbModels, err := db.ListModels()
	if err != nil {
		log.Printf("Error loading models from database: %s", err)
	}
	for _, model := range dbModels {
		modelStore[model.ID] = model
	}

	commands = map[string]Command{
		"/help": func(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, args []string) {
			fmt.Println("Available commands:")
			fmt.Println("  /help - Show this help message")
			fmt.Println("  /list agent - List all registered agents")
			fmt.Println("  /list session - List all created sessions")
			fmt.Println("  /list model - List all registered models")
			fmt.Println("  /add agent @<filename> - Add an agent from a configuration file")
			fmt.Println("  /add model @<filename> - Add a model from a configuration file")
			fmt.Println("  /session start <agent-id> - Create a new agent workload")
			fmt.Println("  /session run [session-id] - Run the current session or a specific session by ID")
			fmt.Println("  /session save - Save the current session")
			fmt.Println("  /quit - Exit the program")
		},
		"/quit": func(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, args []string) {
			os.Exit(0)
		},
		"/session": func(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, args []string) {
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
							Description: agent.Description,
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
						db.AddSession(session)
						workloadChan <- session
						fmt.Printf("Running session with workload ID %s\n", session.Id)
					} else {
						if currentSession != nil {
							inPayloadInputMode = false
							currentSession.Payload = []byte(payloadBuffer.String())
							db.AddSession(currentSession)
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
						db.AddSession(currentSession)
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
		"/list": func(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, args []string) {
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
				case "model":
					if len(modelStore) == 0 {
						fmt.Println("No models registered.")
						return
					}
					for _, model := range modelStore {
						fmt.Printf("  - %s: %s/%s\n", model.ID, model.Provider, model.ModelID)
						if model.APIURL != "" {
							fmt.Printf("    API URL: %s\n", model.APIURL)
						}
						if model.APISpec != "" {
							fmt.Printf("    API Spec: %s\n", model.APISpec)
						}
					}

				default:
					fmt.Println("Unknown subcommand for /list. Try '/list agent', '/list session', or '/list model'")
				}
			} else {
				fmt.Println("Usage: /list <agent|session|model>")
			}
		},
		"/add": func(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, args []string) {
			if len(args) > 0 {
				switch args[0] {
				case "agent":
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
				case "model":
					if len(args) > 1 && strings.HasPrefix(args[1], "@") {
						filename := strings.TrimPrefix(args[1], "@")
						file, err := os.Open(filename)
						if err != nil {
							fmt.Printf("Error opening file: %s\n", err)
							return
						}
						defer file.Close()

						var model models.Model
						decoder := json.NewDecoder(file)
						if err := decoder.Decode(&model); err != nil {
							fmt.Printf("Error decoding model file: %s\n", err)
							return
						}

						if err := db.AddModel(&model); err != nil {
							fmt.Printf("Error adding model to database: %s\n", err)
							return
						}

						modelStore[model.ID] = &model
						fmt.Printf("Model '%s' with ID '%s' added.\n", model.ModelID, model.ID)
					} else {
						fmt.Println("Usage: /add model @<filename>")
					}
				default:
					fmt.Println("Unknown subcommand for /add. Try '/add agent' or '/add model'")
				}
			} else {
				fmt.Println("Usage: /add <agent|model> @<filename>")
			}
		},
	}

	workloadChan := make(chan *pb.Workload)
	// init the workers.
	for _, model := range dbModels {
		if err := worker.Init(context.Background(), model); err != nil {
			log.Fatalf("Error initializing worker for model %s: %s", model.ID, err)
		}
	}

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

		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			if cmd, ok := commands[parts[0]]; ok {
				cmd(db, workloadChan, parts[1:])
			} else {
				fmt.Println("Unknown command. Type /help for a list of commands.")
			}
			continue
		}

		if inPayloadInputMode {
			payloadBuffer.WriteString(input)
			payloadBuffer.WriteString("\n")
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
