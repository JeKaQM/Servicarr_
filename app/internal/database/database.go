package database

import (
	"database/sql"

	// Import SQLite driver for database/sql usage
	_ "modernc.org/sqlite"
)

// DB is the global database instance
var DB *sql.DB

// Querier is implemented by both *sql.DB and *sql.Tx. Helpers that accept it
// can run inside a caller's transaction. Because the pool holds a single
// connection, code inside a transaction must never fall back to DB directly:
// that would wait forever for the connection the transaction is holding.
type Querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

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
