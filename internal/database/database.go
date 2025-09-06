package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

var neo4jDriver neo4j.Driver

type Neo4jConfig struct {
	Uri      string `json:"uri"`
	Username string `json:"username"`
}

func GetNeo4jDriver() (neo4j.Driver, error) {
	if neo4jDriver != nil {
		return neo4jDriver, nil
	}

	configFile, err := os.Open("config.json")
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer configFile.Close()

	var config struct {
		Neo4j Neo4jConfig `json:"neo4j"`
	}
	jsonParser := json.NewDecoder(configFile)
	if err = jsonParser.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	password, err := readPassword("data/neo4j/credentials.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials: %w", err)
	}

	driver, err := neo4j.NewDriver(config.Neo4j.Uri, neo4j.BasicAuth(config.Neo4j.Username, password, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to create neo4j driver: %w", err)
	}

	neo4jDriver = driver
	return neo4jDriver, nil
}

func readPassword(filepath string) (string, error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "password:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "password:")), nil
		}
	}
	return "", fmt.Errorf("password not found in credentials file")
}

func CloseNeo4jDriver() {
	if neo4jDriver != nil {
		neo4jDriver.Close()
	}
}


type Datastore interface {
	AddAgent(agent *models.Agent) error
	GetAgent(id string) (*models.Agent, error)
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
			description TEXT,
			type TEXT
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
			agent_type TEXT,
			models TEXT,
			payload BLOB,
			status TEXT,
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

func (db *SQLiteDatastore) GetAgent(id string) (*models.Agent, error) {
	row := db.db.QueryRow("SELECT id, name, description, type FROM agents WHERE id = ?", id)

	var agent models.Agent
	err := row.Scan(&agent.ID, &agent.Name, &agent.Description, &agent.Type)
	if err != nil {
		return nil, err
	}

	return &agent, nil
}

func (db *SQLiteDatastore) AddAgent(agent *models.Agent) error {
	_, err := db.db.Exec("INSERT INTO agents (id, name, description, type) VALUES (?, ?, ?, ?)", agent.ID, agent.Name, agent.Description, agent.Type)
	return err
}

func (db *SQLiteDatastore) AddSession(session *pb.Workload) error {
	models := strings.Join(session.Models, ",")
	_, err := db.db.Exec("INSERT OR REPLACE INTO sessions (id, name, agent_id, agent_type, models, payload, status) VALUES (?, ?, ?, ?, ?, ?, ?)", session.Id, session.Name, session.AgentId, session.AgentType, models, session.Payload, session.Status.String())
	return err
}

func (db *SQLiteDatastore) GetSession(id string) (*pb.Workload, error) {
	row := db.db.QueryRow("SELECT id, name, agent_id, agent_type, models, payload, status, timestamp FROM sessions WHERE id = ?", id)

	var session pb.Workload
	var timestamp time.Time
	var models string
	var status sql.NullString
	err := row.Scan(&session.Id, &session.Name, &session.AgentId, &session.AgentType, &models, &session.Payload, &status, &timestamp)
	if err != nil {
		return nil, err
	}
	session.Timestamp = timestamp.Unix()
	session.Models = strings.Split(models, ",")
	if status.Valid {
		st, ok := pb.WorkloadStatus_Status_value[status.String]
		if ok {
			session.Status = pb.WorkloadStatus_Status(st)
		}
	}

	return &session, nil
}

func (db *SQLiteDatastore) ListSessions() ([]*pb.Workload, error) {
	rows, err := db.db.Query("SELECT id, name, agent_id, agent_type, models, payload, status, timestamp FROM sessions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*pb.Workload
	for rows.Next() {
		var session pb.Workload
		var timestamp time.Time
		var models string
		var status sql.NullString
		if err := rows.Scan(&session.Id, &session.Name, &session.AgentId, &session.AgentType, &models, &session.Payload, &status, &timestamp); err != nil {
			return nil, err
		}
		session.Timestamp = timestamp.Unix()
		session.Models = strings.Split(models, ",")
		if status.Valid {
			st, ok := pb.WorkloadStatus_Status_value[status.String]
			if ok {
				session.Status = pb.WorkloadStatus_Status(st)
			}
		}
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
	rows, err := s.db.Query("SELECT id, name, description, type FROM agents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		var agent models.Agent
		if err := rows.Scan(&agent.ID, &agent.Name, &agent.Description, &agent.Type); err != nil {
			return nil, err
		}
		agents = append(agents, &agent)
	}

	return agents, nil
}
