package db

import (
	"database/sql"
	_ "embed"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

// Open opens (or creates) the SQLite database at dsn and applies the schema.
//
// Production DSN:  "file:/data/jcg.db?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"
// Test DSN:        "file::memory:?cache=shared&_foreign_keys=on"
func Open(dsn string) (*sql.DB, error) {
	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Single writer avoids SQLITE_BUSY under concurrent requests.
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if _, err := database.Exec(schema); err != nil {
		database.Close()
		return nil, err
	}

	return database, nil
}
