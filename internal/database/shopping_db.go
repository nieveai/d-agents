package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type ShoppingDB struct {
	*sql.DB
}

func NewShoppingDB() (*ShoppingDB, error) {
	db, err := sql.Open("sqlite3", "./shopping.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create the table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS products (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			price REAL,
			date TEXT,
			source TEXT,
			url TEXT
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	return &ShoppingDB{db}, nil
}

func (db *ShoppingDB) InsertProduct(name string, price float64, date time.Time, source string, url string) error {
	_, err := db.Exec(
		"INSERT INTO products (name, price, date, source, url) VALUES (?, ?, ?, ?, ?)",
		name, price, date.Format(time.RFC3339), source, url,
	)
	if err != nil {
		return fmt.Errorf("failed to insert product: %w", err)
	}
	return nil
}

type Product struct {
	ID     int
	Name   string
	Price  float64
	Date   time.Time
	Source string
	URL    sql.NullString
}

func (db *ShoppingDB) GetAllProducts() ([]*Product, error) {
	rows, err := db.Query("SELECT id, name, price, date, source, url FROM products")
	if err != nil {
		return nil, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []*Product
	for rows.Next() {
		var p Product
		var dateStr string
		if err := rows.Scan(&p.ID, &p.Name, &p.Price, &dateStr, &p.Source, &p.URL); err != nil {
			return nil, fmt.Errorf("failed to scan product: %w", err)
		}
		p.Date, err = time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse date: %w", err)
		}
		products = append(products, &p)
	}

	return products, nil
}
