package database

import (
	"database/sql"

	// Import SQLite driver for database/sql usage
	_ "modernc.org/sqlite"
)

// DB is the global database instance
var DB *sql.DB

// Init initializes the database connection and creates schema
func Init(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	// SQLite tuning for production
	DB.SetMaxOpenConns(1) // SQLite only supports one writer at a time
	DB.SetMaxIdleConns(1)
	DB.SetConnMaxLifetime(0)             // Connections don't expire
	DB.Exec("PRAGMA journal_mode=WAL")   // Write-Ahead Logging for better concurrency
	DB.Exec("PRAGMA busy_timeout=5000")  // Wait up to 5s when database is locked
	DB.Exec("PRAGMA synchronous=NORMAL") // Safe with WAL mode, better performance
	DB.Exec("PRAGMA foreign_keys=ON")    // Enforce foreign key constraints

	return EnsureSchema()
}
