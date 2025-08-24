package database

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nieveai/d-agents/internal/models"
)

type Datastore interface {
	AddAgent(agent *models.Agent) error
	ListAgents() ([]*models.Agent, error)
}

type SQLiteDatastore struct {
	db *sql.DB
}

func NewSQLiteDatastore(dataSourceName string) (*SQLiteDatastore, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (
			id TEXT NOT NULL PRIMARY KEY,
			name TEXT,
			description TEXT
		);
	`); err != nil {
		return nil, err
	}

	return &SQLiteDatastore{db: db}, nil
}

func (s *SQLiteDatastore) AddAgent(agent *models.Agent) error {
	_, err := s.db.Exec("INSERT INTO agents (id, name, description) VALUES (?, ?, ?)", agent.ID, agent.Name, agent.Description)
	return err
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
