package db

import "database/sql"

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	return nil, nil
}

func (d *DB) Migrate() error {
	return nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}
