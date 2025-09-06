package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/google/uuid"
	"github.com/nieveai/d-agents/internal/database"
	amodels "github.com/nieveai/d-agents/internal/models"
	"github.com/nieveai/d-agents/internal/worker"
	pb "github.com/nieveai/d-agents/proto"
)

type Config struct {
	Workers int `json:"workers"`
}

var modelStore = make(map[string]*amodels.Model)
var sessions = make(map[string]*pb.Workload)
var openSessionTabs = make(map[string]*container.TabItem)
var scheduledSessions = make(map[string]*time.Ticker)
var currentSession *pb.Workload

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

	workloadChan := make(chan *pb.Workload)
	refreshChan := make(chan bool, 1)
	// init the workers.
	if err := worker.Init(context.Background(), dbModels, db); err != nil {
		log.Fatalf("Error initializing worker: %s", err)
	}

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		go runWorker(i, workloadChan)
	}

	a := app.New()
	w := a.NewWindow("D-Agents Controller")

	tabs := container.NewAppTabs()
	tabs.Append(container.NewTabItem("Agents", makeAgentsTab(db, w)))
	tabs.Append(container.NewTabItem("Models", makeModelsTab(db, w)))
	tabs.Append(container.NewTabItem("Sessions", makeSessionsTab(db, tabs, workloadChan, w, refreshChan)))

	w.SetContent(tabs)
	w.Resize(fyne.NewSize(1000, 800))
	w.ShowAndRun()
}

func makeAgentsTab(db *database.SQLiteDatastore, window fyne.Window) fyne.CanvasObject {
	agents, err := db.ListAgents()
	if err != nil {
		log.Printf("Error loading agents from database: %s", err)
	}

	list := widget.NewList(
		func() int {
			return len(agents)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(agents[i].Name)
		},
	)

	addButton := widget.NewButton("Add Agent", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			if reader == nil {
				return
			}
			defer reader.Close()

			var agent amodels.Agent
			decoder := json.NewDecoder(reader)
			if err := decoder.Decode(&agent); err != nil {
				dialog.ShowError(err, window)
				return
			}

			if err := db.AddAgent(&agent); err != nil {
				dialog.ShowError(err, window)
				return
			}

			// Refresh the list
			newAgents, err := db.ListAgents()
			if err != nil {
				log.Printf("Error loading agents from database: %s", err)
			} else {
				agents = newAgents
				list.Refresh()
			}
		}, window)
	})

	return container.NewBorder(nil, addButton, nil, nil, list)
}

func makeModelsTab(db *database.SQLiteDatastore, window fyne.Window) fyne.CanvasObject {
	models, err := db.ListModels()
	if err != nil {
		log.Printf("Error loading models from database: %s", err)
	}

	list := widget.NewList(
		func() int {
			return len(models)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(models[i].ModelID)
		},
	)

	addButton := widget.NewButton("Add Model", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			if reader == nil {
				return
			}
			defer reader.Close()

			var model amodels.Model
			decoder := json.NewDecoder(reader)
			if err := decoder.Decode(&model); err != nil {
				dialog.ShowError(err, window)
				return
			}

			if err := db.AddModel(&model); err != nil {
				dialog.ShowError(err, window)
				return
			}

			// Refresh the list
			newModels, err := db.ListModels()
			if err != nil {
				log.Printf("Error loading models from database: %s", err)
			} else {
				models = newModels
				list.Refresh()
			}
		}, window)
	})

	return container.NewBorder(nil, addButton, nil, nil, list)
}

func makeSessionsTab(db *database.SQLiteDatastore, tabs *container.AppTabs, workloadChan chan<- *pb.Workload, window fyne.Window, refreshChan chan bool) fyne.CanvasObject {
	sessions, err := db.ListSessions()
	if err != nil {
		log.Printf("Error loading sessions from database: %s", err)
	}

	columnWidths := []float32{150, 100, 250, 300, 50}
	var table *widget.Table
	table = widget.NewTable(
		func() (int, int) {
			return len(sessions) + 1, 5 // Add 1 for header row, 5 columns
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			label := o.(*widget.Label)
			if id.Row == 0 {
				// Header row
				switch id.Col {
				case 0:
					label.SetText("Name")
				case 1:
					label.SetText("Status")
				case 2:
					label.SetText("Timestamp")
				case 3:
					label.SetText("Payload")
				case 4:
					label.SetText("Action")
				}
				return
			}

			// Data rows
			if id.Col == 3 { // Payload column
				label.Wrapping = fyne.TextWrapWord
			} else { // Other columns
				label.Wrapping = fyne.TextWrapOff
			}
			session := sessions[id.Row-1]
			switch id.Col {
			case 0:
				label.SetText(session.Name)
			case 1:
				label.SetText(session.Status.String())
			case 2:
				label.SetText(time.Unix(session.Timestamp, 0).Format(time.RFC1123))
			case 3:
				payload := string(session.Payload)
				if len(payload) > 100 {
					payload = payload[:100] + "..."
				}
				label.SetText(payload)

				// Calculate required height for wrapped text
				tempLabel := widget.NewLabel(payload)
				tempLabel.Wrapping = fyne.TextWrapWord
				tempLabel.Resize(fyne.NewSize(columnWidths[id.Col], 0))
				requiredHeight := tempLabel.MinSize().Height

				table.SetRowHeight(id.Row, requiredHeight)

			case 4:
				label.SetText("Load")

			}
		},
	)
	for i, width := range columnWidths {
		table.SetColumnWidth(i, width)
	}

	table.OnSelected = func(id widget.TableCellID) {
		if id.Row > 0 && id.Col == 4 {
			session := sessions[id.Row-1]
			if tab, ok := openSessionTabs[session.Id]; ok {
				tabs.Select(tab)
			} else {
				tab := container.NewTabItem(session.Name, nil)
				tab.Content = makeSessionTab(session, db, workloadChan, refreshChan, tabs, tab, window)
				openSessionTabs[session.Id] = tab
				tabs.Append(tab)
				tabs.Select(tab)
			}
		}
		table.Unselect(id)
	}

	go func(table *widget.Table, sessions *[]*pb.Workload) {
		for range refreshChan {
			newSessions, err := db.ListSessions()
			if err != nil {
				log.Printf("Error loading sessions from database: %s", err)
				continue
			}
			*sessions = newSessions
			fyne.Do(func() {
				table.Refresh()
			})
		}
	}(table, &sessions)

	createButton := widget.NewButton("Create Session", func() {
		agents, err := db.ListAgents()
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		models, err := db.ListModels()
		if err != nil {
			dialog.ShowError(err, window)
			return
		}

		selectedAgent := agents[0]
		selectedModels := []*amodels.Model{}
		sessionNameEntry := widget.NewEntry()
		sessionNameEntry.SetPlaceHolder("Enter session name...")

		agentSelect := widget.NewSelect(agentNames(agents), func(s string) {
			for _, a := range agents {
				if a.Name == s {
					selectedAgent = a
					break
				}
			}
		})
		agentSelect.SetSelected(selectedAgent.Name)

		modelCheck := widget.NewCheckGroup(modelNames(models), func(ss []string) {
			selectedModels = []*amodels.Model{}
			for _, s := range ss {
				for _, m := range models {
					if m.ModelID == s {
						selectedModels = append(selectedModels, m)
					}
				}
			}
		})

		d := dialog.NewForm("Create Session", "Create", "Cancel", []*widget.FormItem{
			widget.NewFormItem("Session Name", sessionNameEntry),
			widget.NewFormItem("Agent", agentSelect),
			widget.NewFormItem("Models", modelCheck),
		}, func(b bool) {
			if !b {
				return
			}

			modelIDs := []string{}
			for _, m := range selectedModels {
				modelIDs = append(modelIDs, m.ID)
			}

			sessionName := sessionNameEntry.Text
			if sessionName == "" {
				sessionName = selectedAgent.Name
			}

			newSession := &pb.Workload{
				Id:        uuid.New().String(),
				Name:      sessionName,
				AgentId:   selectedAgent.ID,
				AgentType: selectedAgent.Type,
				Models:    modelIDs,
				Timestamp: time.Now().Unix(),
				Status:    pb.WorkloadStatus_PENDING,
			}
			tab := container.NewTabItem(newSession.Name, nil)
			tab.Content = makeSessionTab(newSession, db, workloadChan, refreshChan, tabs, tab, window)
			openSessionTabs[newSession.Id] = tab
			tabs.Append(tab)
			tabs.Select(tab)
		}, window)

		d.Show()
		window.Canvas().Focus(sessionNameEntry)
	})

	refreshButton := widget.NewButton("Refresh", func() {
		refreshChan <- true
	})

	return container.NewBorder(nil, container.NewHBox(createButton, refreshButton), nil, nil, table)
}

func makeSessionTab(session *pb.Workload, db *database.SQLiteDatastore, workloadChan chan<- *pb.Workload, refreshChan chan bool, tabs *container.AppTabs, tab *container.TabItem, window fyne.Window) fyne.CanvasObject {
	label := widget.NewLabel(fmt.Sprintf("Session: %s", session.Name))
	statusLabel := widget.NewLabel(fmt.Sprintf("Status: %s Agent: %s Models: %s", session.Status.String(), session.AgentId, session.Models))
	done := make(chan struct{})

	closeButton := widget.NewButton("X", func() {
		if ticker, ok := scheduledSessions[session.Id]; ok {
			ticker.Stop()
			delete(scheduledSessions, session.Id)
		}
		close(done)
		tabs.Remove(tab)
		delete(openSessionTabs, session.Id)
	})

	// View mode widgets
	richText := widget.NewRichTextFromMarkdown(string(session.Payload))
	richText.Wrapping = fyne.TextWrapWord
	viewScroll := container.NewScroll(richText)

	// Edit mode widgets
	payloadBinding := binding.NewString()
	payloadBinding.Set(string(session.Payload))
	payloadEntry := widget.NewEntryWithData(payloadBinding)
	payloadEntry.MultiLine = true
	editScroll := container.NewScroll(payloadEntry)

	var editButton, saveButton, runButton, stopButton *widget.Button

	runSession := func() {
		text, _ := payloadBinding.Get()
		session.Payload = []byte(text)
		session.Status = pb.WorkloadStatus_RUNNING
		db.AddSession(session)
		richText.ParseMarkdown(string(session.Payload))
		statusLabel.SetText(fmt.Sprintf("Status: %s Agent: %s Models: %s", session.Status.String(), session.AgentId, session.Models))
		workloadChan <- session
		refreshChan <- true
	}

	// Toggling logic
	showViewMode := func() {
		viewScroll.Show()
		editScroll.Hide()
		editButton.Show()
		saveButton.Hide()
		runButton.Show()
		if _, ok := scheduledSessions[session.Id]; ok {
			stopButton.Show()
			runButton.Hide()
		} else {
			stopButton.Hide()
			runButton.Show()
		}
	}

	showEditMode := func() {
		viewScroll.Hide()
		editScroll.Show()
		editButton.Hide()
		saveButton.Show()
		runButton.Show()
		stopButton.Hide()
	}

	var startPolling func()
	startPolling = func() {
		go func() {
			if session.Status != pb.WorkloadStatus_RUNNING {
				return
			}

			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					log.Printf("Checking status for session %s", session.Id)
					newSession, err := db.GetSession(session.Id)
					if err != nil {
						log.Printf("Error checking session %s: %s", session.Id, err)
						continue
					}

					if newSession.Status != pb.WorkloadStatus_RUNNING {
						session.Status = newSession.Status
						statusLabel.SetText(fmt.Sprintf("Status: %s Agent: %s Models: %s", session.Status.String(), session.AgentId, session.Models))

						if newSession.Status == pb.WorkloadStatus_COMPLETED {
							log.Printf("Session %s completed. Reloading payload.", session.Id)
							session.Payload = newSession.Payload
							richText.ParseMarkdown(string(session.Payload))
							payloadBinding.Set(string(session.Payload))
						}
						return // Stop polling
					}
				case <-done:
					log.Printf("Stopping refresh for session %s", session.Id)
					return
				}
			}
		}()
	}

	editButton = widget.NewButton("Edit", showEditMode)
	saveButton = widget.NewButton("Save", func() {
		text, _ := payloadBinding.Get()
		session.Payload = []byte(text)
		db.AddSession(session)
		richText.ParseMarkdown(string(session.Payload))
		showViewMode()
		refreshChan <- true
	})
	runButton = widget.NewButton("Run", func() {
		intervalEntry := widget.NewEntry()
		intervalEntry.SetPlaceHolder("e.g., 1, 2.5")
		intervalEntry.Disable()

		scheduleCheck := widget.NewCheck("Schedule periodic runs", func(checked bool) {
			if checked {
				intervalEntry.Enable()
			} else {
				intervalEntry.Disable()
			}
		})

		formItems := []*widget.FormItem{
			widget.NewFormItem("", scheduleCheck),
			widget.NewFormItem("Interval (hours)", intervalEntry),
		}

		dialog.ShowForm("Run Session", "Run", "Cancel", formItems, func(b bool) {
			if !b {
				return
			}

			if !scheduleCheck.Checked {
				// Run immediately
				runSession()
				startPolling()
				showViewMode()
				return
			}

			// Schedule run
			intervalStr := intervalEntry.Text
			if intervalStr == "" {
				dialog.ShowError(fmt.Errorf("interval cannot be empty for scheduled run"), window)
				return
			}

			interval, err := time.ParseDuration(intervalStr + "h")
			if err != nil {
				dialog.ShowError(fmt.Errorf("invalid interval: %w", err), window)
				return
			}

			ticker := time.NewTicker(interval)
			scheduledSessions[session.Id] = ticker
			go func() {
				for {
					select {
					case <-ticker.C:
						if session.Status == pb.WorkloadStatus_RUNNING {
							log.Printf("Session %s is already running. Skipping scheduled run.", session.Id)
							continue
						}
						runSession()
						startPolling()
					case <-done:
						return
					}
				}
			}()
			statusLabel.SetText(fmt.Sprintf("Status: Scheduled every %s Agent: %s Models: %s", interval, session.AgentId, session.Models))
			showViewMode()
		}, window)
	})

	stopButton = widget.NewButton("Stop", func() {
		if ticker, ok := scheduledSessions[session.Id]; ok {
			ticker.Stop()
			delete(scheduledSessions, session.Id)
			statusLabel.SetText(fmt.Sprintf("Status: %s Agent: %s Models: %s", session.Status.String(), session.AgentId, session.Models))
			showViewMode()
		}
	})

	buttonContainer := container.NewHBox(editButton, saveButton, runButton, stopButton)

	content := container.NewStack(viewScroll, editScroll)

	showViewMode()
	startPolling()

	return container.NewBorder(
		container.NewBorder(nil, nil, nil, container.NewHBox(buttonContainer, closeButton), label),
		statusLabel,
		nil,
		nil,
		content,
	)
}

func agentNames(agents []*amodels.Agent) []string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	return names
}

func modelNames(models []*amodels.Model) []string {
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.ModelID
	}
	return names
}

func runWorker(id int, workloadChan <-chan *pb.Workload) {
	for workload := range workloadChan {
		log.Printf("Worker %d processing workload: %s", id, workload.Id)
		worker.ProcessWorkload(workload)
	}
	log.Printf("Worker %d shutting down", id)
}
