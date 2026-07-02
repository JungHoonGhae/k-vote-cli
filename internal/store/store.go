// Package store persists normalized 선거 데이터 (개표결과·여론조사) into a local
// SQLite database and exposes read-only SQL query access. It performs no network
// I/O and no interpretation — it stores 원자료 verbatim and derives only the
// standard, defined parameters (뷰 정의 참조). 판단은 소비자의 몫.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a *sql.DB opened against a kvote SQLite file.
type DB struct{ db *sql.DB }

// SQL exposes the underlying handle for queries and tests.
func (d *DB) SQL() *sql.DB { return d.db }

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

// DefaultPath returns the OS-conventional kvote DB location.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kvote", "kvote.db"), nil
}

// Open opens (creating if absent) a writable kvote DB and ensures the schema.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	sdb, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(true)")
	if err != nil {
		return nil, err
	}
	d := &DB{db: sdb}
	if err := d.migrate(); err != nil {
		sdb.Close()
		return nil, err
	}
	return d, nil
}

// OpenReadOnly opens an existing kvote DB in read-only mode; writes are rejected
// by the engine itself, so no SQL filtering is needed.
func OpenReadOnly(path string) (*DB, error) {
	sdb, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=query_only(true)")
	if err != nil {
		return nil, err
	}
	return &DB{db: sdb}, nil
}

// migrate applies SchemaSQL when the DB is new or matches the current version.
// On an incompatible version it errors advising recreation (no migration chain in v1).
func (d *DB) migrate() error {
	var v int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v != 0 && v != schemaVersion {
		return fmt.Errorf("db schema version %d != %d (지원 안 함) — DB 파일을 삭제하고 재생성하세요", v, schemaVersion)
	}
	if _, err := d.db.Exec(SchemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if _, err := d.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return err
	}
	return nil
}
