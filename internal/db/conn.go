package db

import (
	"database/sql"
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	// Expand leading ~ to actual home directory.
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, path[2:])
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// Enable WAL mode and foreign keys.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			conn.Close()
			return nil, err
		}
	}

	return &DB{conn: conn}, nil
}

func (d *DB) Migrate() error {
	_, err := d.conn.Exec(schema)
	return err
}

func (d *DB) Conn() *sql.DB {
	return d.conn
}

func (d *DB) Close() error {
	return d.conn.Close()
}
