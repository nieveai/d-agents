package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

type model struct {
	viewport     viewport.Model
	messages     []string
	textarea     textarea.Model
	senderStyle  lipgloss.Style
	err          error
	db           *database.SQLiteDatastore
	workloadChan chan<- *pb.Workload
}

type responseMsg string

func initialModel(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload) *model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = 280

	ta.SetWidth(50)
	ta.SetHeight(1)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	ta.ShowLineNumbers = false

	vp := viewport.New(50, 5)
	vp.SetContent(`Welcome to the d-agents controller!
Type a command to get started. (e.g. /help)`)

	ta.KeyMap.InsertNewline.SetEnabled(false)

	return &model{
		textarea:     ta,
		messages:     []string{},
		viewport:     vp,
		senderStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:          nil,
		db:           db,
		workloadChan: workloadChan,
	}
}

func (m *model) Init() tea.Cmd {
	return textarea.Blink
}

func (m *model) processCommand() {
	input := m.textarea.Value()
	if strings.HasPrefix(input, "/") {
		parts := strings.Fields(input)
		if cmd, ok := commands[parts[0]]; ok {
			cmd(m.db, m.workloadChan, parts[1:])
		} else {
			m.messages = append(m.messages, "Unknown command. Type /help for a list of commands.")
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
		}
	} else {
		if inPayloadInputMode {
			payloadBuffer.WriteString(input)
			payloadBuffer.WriteString("\n")
		} else {
			m.messages = append(m.messages, "Invalid command. Please use the format 'type payload' or start a session.")
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
		}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case tea.KeyEnter:
			m.messages = append(m.messages, m.senderStyle.Render("You: ")+m.textarea.Value())
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.processCommand() // Call the command processing logic
			m.textarea.Reset()
			m.viewport.GotoBottom()
		}
	case responseMsg:
		m.messages = append(m.messages, string(msg))
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()

	// We handle errors just like any other message
	case error:
		m.err = msg
		return m, nil
	}

		return m, tea.Batch(tiCmd, vpCmd)
}

func (m *model) View() string {
	return fmt.Sprintf(
		"%s\n\n%s",
		m.viewport.View(),
		m.textarea.View(),
	)
}

var p *tea.Program

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
			helpText := `Available commands:
  /help - Show this help message
  /list agent - List all registered agents
  /list session - List all created sessions
  /list model - List all registered models
  /add agent @<filename> - Add an agent from a configuration file
  /add model @<filename> - Add a model from a configuration file
  /session start <agent-id> <model-id1,model-id2,...> - Create a new agent workload
  /session run [session-id] - Run the current session or a specific session by ID
  /session save - Save the current session
  /session load <workload-id> - Load a session by ID
  /quit - Exit the program`
			p.Send(responseMsg(helpText))
		},
		"/quit": func(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, args []string) {
			os.Exit(0)
		},
		"/session": func(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, args []string) {
			if len(args) > 0 {
				switch args[0] {
				case "start":
					if len(args) > 2 {
						agentID := args[1]
						modelIDsRaw := args[2]
						agent, ok := agents[agentID]
						if !ok {
							p.Send(responseMsg(fmt.Sprintf("Agent with ID '%s' not found.", agentID)))
							return
						}

						modelIDs := strings.Split(modelIDsRaw, ",")
						for _, modelID := range modelIDs {
							if _, ok := modelStore[modelID]; !ok {
								p.Send(responseMsg(fmt.Sprintf("Model with ID '%s' not found.", modelID)))
								return
							}
						}

						workloadID := uuid.New().String()
						workload := &pb.Workload{
							Id:          workloadID,
							Name:        agent.Name,
							Models:      modelIDs,
							Description: agent.Description,
							AgentId:     agent.ID,
							AgentType:   agent.Type,
							Timestamp:   time.Now().Unix(),
							Status:      pb.WorkloadStatus_PENDING,
						}

						sessions[workloadID] = workload
						currentSession = workload
						inPayloadInputMode = true
						payloadBuffer.Reset()
						p.Send(responseMsg("what would you like the agent to do? Please enter your instruction below."))
					} else {
						p.Send(responseMsg("Usage: /session start <agent-id> <model-id1,model-id2,...>"))
					}

				case "run":
					if len(args) > 1 {
						sessionID := args[1]
						session, ok := sessions[sessionID]
						if !ok {
							p.Send(responseMsg(fmt.Sprintf("Session with ID '%s' not found.", sessionID)))
							return
						}
						session.Status = pb.WorkloadStatus_RUNNING
						db.AddSession(session)
						workloadChan <- session
						p.Send(responseMsg(fmt.Sprintf("Running session with workload ID %s", session.Id)))
					} else {
						if currentSession != nil {
							inPayloadInputMode = false
							payload := payloadBuffer.String()
							payloadBuffer.Reset()

							currentSession.Payload = []byte(payload)
							currentSession.Status = pb.WorkloadStatus_RUNNING
							db.AddSession(currentSession)
							workloadChan <- currentSession
							p.Send(responseMsg(fmt.Sprintf("Running session with workload ID %s", currentSession.Id)))
						} else {
							p.Send(responseMsg("No active session. Use '/session start <agent-id>' to start one."))
						}
					}

				case "save":
					if currentSession != nil {
						inPayloadInputMode = false
						payload := payloadBuffer.String()

						currentSession.Payload = []byte(payload)
						db.AddSession(currentSession)
						sessions[currentSession.Id] = currentSession
						p.Send(responseMsg(fmt.Sprintf("Saved session with workload ID %s", currentSession.Id)))
					} else {
						p.Send(responseMsg("No active session. Use '/session start <agent-id>' to start one."))
					}
				case "load":
					if len(args) > 1 {
						sessionID := args[1]
						session, err := db.GetSession(sessionID)
						if err != nil {
							p.Send(responseMsg(fmt.Sprintf("Error loading session: %s", err)))
							return
						}
						if session == nil {
							p.Send(responseMsg(fmt.Sprintf("Session with ID '%s' not found.", sessionID)))
							return
						}
						currentSession = session
						sessions[session.Id] = session
						payloadBuffer.Reset()
						payloadBuffer.Write(session.Payload)
						inPayloadInputMode = true
						p.Send(responseMsg(fmt.Sprintf("Loaded session with ID: %s\nPayload:\n%s", session.Id, string(session.Payload))))
					} else {
						p.Send(responseMsg("Usage: /session load <workload-id>"))
					}
				default:
					p.Send(responseMsg("Unknown command for /session. Available commands: start, run, save, load"))
				}
			} else {
				p.Send(responseMsg("Usage: /session <start|run|save|load>"))
			}
		},
		"/list": func(db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, args []string) {
			if len(args) > 0 {
				switch args[0] {
				case "agent":
					if len(agents) == 0 {
						p.Send(responseMsg("No agents registered."))
						return
					}
					var builder strings.Builder
					for _, agent := range agents {
						builder.WriteString(fmt.Sprintf("  - %s: %s (%s)\n    Description: %s\n", agent.ID, agent.Name, agent.Type, agent.Description))
					}
					p.Send(responseMsg(builder.String()))

				case "session":
					dbSessions, err := db.ListSessions()
					if err != nil {
						p.Send(responseMsg(fmt.Sprintf("Error loading sessions from database: %s", err)))
						return
					}
					if len(dbSessions) == 0 {
						p.Send(responseMsg("No sessions created."))
						return
					}
					var builder strings.Builder
					for _, session := range dbSessions {
						payload := string(session.Payload)
						if len(payload) > 50 {
							payload = payload[:50] + "..."
						}
						builder.WriteString(fmt.Sprintf("  - %s: %s (%s)\n    Payload: %s\n", session.Id, session.Name, session.Status, payload))
					}
					p.Send(responseMsg(builder.String()))

				case "model":
					if len(modelStore) == 0 {
						p.Send(responseMsg("No models registered."))
						return
					}
					var builder strings.Builder
					for _, model := range modelStore {
						builder.WriteString(fmt.Sprintf("  - %s: %s/%s\n", model.ID, model.Provider, model.ModelID))
						if model.APIURL != "" {
							builder.WriteString(fmt.Sprintf("    API URL: %s\n", model.APIURL))
						}
						if model.APISpec != "" {
							builder.WriteString(fmt.Sprintf("    API Spec: %s\n", model.APISpec))
						}
					}
					p.Send(responseMsg(builder.String()))

				default:
					p.Send(responseMsg("Unknown subcommand for /list. Try '/list agent', '/list session', or '/list model'"))
				}
			} else {
				p.Send(responseMsg("Usage: /list <agent|session|model>"))
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
							p.Send(responseMsg(fmt.Sprintf("Error opening file: %s", err)))
							return
						}
						defer file.Close()

						var agent models.Agent
						decoder := json.NewDecoder(file)
						if err := decoder.Decode(&agent); err != nil {
							p.Send(responseMsg(fmt.Sprintf("Error decoding agent file: %s", err)))
							return
						}

						if err := db.AddAgent(&agent); err != nil {
							p.Send(responseMsg(fmt.Sprintf("Error adding agent to database: %s", err)))
							return
						}

						agents[agent.ID] = &agent
						p.Send(responseMsg(fmt.Sprintf("Agent '%s' with ID '%s' added.", agent.Name, agent.ID)))
					} else {
						p.Send(responseMsg("Usage: /add agent @<filename>"))
					}
				case "model":
					if len(args) > 1 && strings.HasPrefix(args[1], "@") {
						filename := strings.TrimPrefix(args[1], "@")
						file, err := os.Open(filename)
						if err != nil {
							p.Send(responseMsg(fmt.Sprintf("Error opening file: %s", err)))
							return
						}
						defer file.Close()

						var model models.Model
						decoder := json.NewDecoder(file)
						if err := decoder.Decode(&model); err != nil {
							p.Send(responseMsg(fmt.Sprintf("Error decoding model file: %s", err)))
							return
						}

						if err := db.AddModel(&model); err != nil {
							p.Send(responseMsg(fmt.Sprintf("Error adding model to database: %s", err)))
							return
						}

						modelStore[model.ID] = &model
						p.Send(responseMsg(fmt.Sprintf("Model '%s' with ID '%s' added.", model.ModelID, model.ID)))
					} else {
						p.Send(responseMsg("Usage: /add model @<filename>"))
					}
				default:
					p.Send(responseMsg("Unknown subcommand for /add. Try '/add agent' or '/add model'"))
				}
			} else {
				p.Send(responseMsg("Usage: /add <agent|model> @<filename>"))
			}
		},
	}

	workloadChan := make(chan *pb.Workload)
	// init the workers.
	if err := worker.Init(context.Background(), dbModels, db); err != nil {
		log.Fatalf("Error initializing worker: %s", err)
	}

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		go runWorker(i, workloadChan)
	}

	p = tea.NewProgram(initialModel(db, workloadChan))

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

func runWorker(id int, workloadChan <-chan *pb.Workload) {
	for workload := range workloadChan {
		log.Printf("Worker %d processing workload: %s", id, strings.Join(workload.Models, ","))
		worker.ProcessWorkload(workload)
	}
	log.Printf("Worker %d shutting down", id)
}
