package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/jmoiron/sqlx"
)

// DB holds the database connection
var DB *sqlx.DB

// Config holds database configuration
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// Connect establishes a connection to PostgreSQL
func Connect(config Config) (*sqlx.DB, error) {
	// Validate DBName is not empty before building DSN
	if config.DBName == "" {
		return nil, fmt.Errorf("database name (dbname) is empty in configuration")
	}

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host,
		config.Port,
		config.User,
		config.Password,
		config.DBName,
		config.SSLMode,
	)

	// Log DSN for debugging (without password)
	log.Printf("[DB] Connecting to database: host=%s port=%d user=%s dbname='%s' sslmode=%s", 
		config.Host, config.Port, config.User, config.DBName, config.SSLMode)
	log.Printf("[DB] DSN (password hidden): %s", 
		fmt.Sprintf("host=%s port=%d user=%s password=*** dbname=%s sslmode=%s",
			config.Host, config.Port, config.User, config.DBName, config.SSLMode))

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	DB = db
	return db, nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// GetDB returns the database connection
func GetDB() *sqlx.DB {
	return DB
}

// BeginTx starts a new transaction
func BeginTx() (*sql.Tx, error) {
	if DB == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}
	return DB.Begin()
}

