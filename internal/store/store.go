// Package store implements the SQLite persistence boundary for the pile-pouring
// quality-closure backend. It owns the schema, migrations, transactional access,
// unique/foreign/check constraints, optimistic versioning and restart recovery
// described in component 6 of the traceability matrix.
//
// All access runs through a Tx. The single-connection pool makes transaction
// ordering deterministic, which the public black-box tests rely on.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo)
)

// SQLStore is the concrete persistence implementation.
type SQLStore struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, applies migrations and
// verifies connectivity. The DSN enables foreign keys, a busy timeout and WAL
// journaling so cross-process restarts recover cleanly.
func Open(path string) (*SQLStore, error) {
	dsn := "file:" + url.PathEscape(path) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// A single connection serialises writes deterministically; SQLite's own
	// locking plus our optimistic version checks provide the safety the
	// single-writer barrier and balance-conservation invariants require.
	db.SetMaxOpenConns(1)
	s := &SQLStore{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Ping reports whether the database is reachable.
func (s *SQLStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close releases the underlying connection pool.
func (s *SQLStore) Close() error {
	return s.db.Close()
}

// Begin starts a transaction for a unit of business work.
func (s *SQLStore) Begin(ctx context.Context) (*Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin: %w", err)
	}
	return &Tx{tx: tx}, nil
}

// Tx is a scoped transaction exposing all persistence operations.
type Tx struct {
	tx *sql.Tx
}

// Commit persists the transaction's writes.
func (t *Tx) Commit() error { return t.tx.Commit() }

// Rollback discards the transaction's writes.
func (t *Tx) Rollback() error { return t.tx.Rollback() }
