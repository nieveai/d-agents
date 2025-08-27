package database

import (
	"database/sql"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

type Datastore interface {
	AddAgent(agent *models.Agent) error
	ListAgents() ([]*models.Agent, error)
	AddSession(session *pb.Workload) error
	GetSession(id string) (*pb.Workload, error)
	ListSessions() ([]*pb.Workload, error)
	AddModel(model *models.Model) error
	GetModel(id string) (*models.Model, error)
	ListModels() ([]*models.Model, error)
}

type SQLiteDatastore struct {
	db *sql.DB
}

func NewSQLiteDatastore(path string) (*SQLiteDatastore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// Create agents table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT
		);
	`)
	if err != nil {
		return nil, err
	}

	// Create sessions table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT,
			agent_id TEXT,
			models TEXT,
			payload BLOB,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return nil, err
	}

	// Create models table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS models (
			id TEXT PRIMARY KEY,
			provider TEXT,
			api_key TEXT,
			model_id TEXT,
			api_url TEXT,
			api_spec TEXT
		);
	`)
	if err != nil {
		return nil, err
	}

	return &SQLiteDatastore{db: db}, nil
}

func (db *SQLiteDatastore) AddAgent(agent *models.Agent) error {
	_, err := db.db.Exec("INSERT INTO agents (id, name, description) VALUES (?, ?, ?)", agent.ID, agent.Name, agent.Description)
	return err
}

func (db *SQLiteDatastore) AddSession(session *pb.Workload) error {
	models := strings.Join(session.Models, ",")
	_, err := db.db.Exec("INSERT OR REPLACE INTO sessions (id, name, agent_id, models, payload) VALUES (?, ?, ?, ?, ?)", session.Id, session.Name, session.AgentId, models, session.Payload)
	return err
}

func (db *SQLiteDatastore) GetSession(id string) (*pb.Workload, error) {
	row := db.db.QueryRow("SELECT id, name, agent_id, models, payload, timestamp FROM sessions WHERE id = ?", id)

	var session pb.Workload
	var timestamp time.Time
	var models string
	err := row.Scan(&session.Id, &session.Name, &session.AgentId, &models, &session.Payload, &timestamp)
	if err != nil {
		return nil, err
	}
	session.Timestamp = timestamp.Unix()
	session.Models = strings.Split(models, ",")

	return &session, nil
}

func (db *SQLiteDatastore) ListSessions() ([]*pb.Workload, error) {
	rows, err := db.db.Query("SELECT id, name, agent_id, models, payload, timestamp FROM sessions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*pb.Workload
	for rows.Next() {
		var session pb.Workload
		var timestamp time.Time
		var models string
		if err := rows.Scan(&session.Id, &session.Name, &session.AgentId, &models, &session.Payload, &timestamp); err != nil {
			return nil, err
		}
		session.Timestamp = timestamp.Unix()
		session.Models = strings.Split(models, ",")
		sessions = append(sessions, &session)
	}

	return sessions, nil
}

func (db *SQLiteDatastore) AddModel(model *models.Model) error {
	_, err := db.db.Exec("INSERT INTO models (id, provider, api_key, model_id, api_url, api_spec) VALUES (?, ?, ?, ?, ?, ?)", model.ID, model.Provider, model.APIKey, model.ModelID, model.APIURL, model.APISpec)
	return err
}

func (db *SQLiteDatastore) GetModel(id string) (*models.Model, error) {
	row := db.db.QueryRow("SELECT id, provider, api_key, model_id, api_url, api_spec FROM models WHERE id = ?", id)

	var model models.Model
	err := row.Scan(&model.ID, &model.Provider, &model.APIKey, &model.ModelID, &model.APIURL, &model.APISpec)
	if err != nil {
		return nil, err
	}

	return &model, nil
}

func (db *SQLiteDatastore) ListModels() ([]*models.Model, error) {
	rows, err := db.db.Query("SELECT id, provider, api_key, model_id, api_url, api_spec FROM models")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models_list []*models.Model
	for rows.Next() {
		var model models.Model
		if err := rows.Scan(&model.ID, &model.Provider, &model.APIKey, &model.ModelID, &model.APIURL, &model.APISpec); err != nil {
			return nil, err
		}
		models_list = append(models_list, &model)
	}

	return models_list, nil
}

func (s *SQLiteDatastore) ListAgents() ([]*models.Agent, error) {
	rows, err := s.db.Query("SELECT id, name, description FROM agents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		var agent models.Agent
		if err := rows.Scan(&agent.ID, &agent.Name, &agent.Description); err != nil {
			return nil, err
		}
		agents = append(agents, &agent)
	}

	return agents, nil
}
