package store

import (
	"context"
	"fmt"
)

// migrations is the ordered list of schema version statements. Each entry runs
// inside the same transaction when the schema version is lower than the entry's
// index. Versions are append-only; existing databases are upgraded in place.
var migrations = []string{
	// v1: core entities.
	`
CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	stage TEXT NOT NULL,
	locked_version INTEGER NOT NULL DEFAULT 0,
	generation INTEGER NOT NULL DEFAULT 0,
	last_time INTEGER NOT NULL DEFAULT 0,
	age_deadline INTEGER NOT NULL DEFAULT 0,
	version INTEGER NOT NULL DEFAULT 0,
	terminal TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS designs (
	pile_id TEXT PRIMARY KEY REFERENCES tasks(id),
	generation INTEGER NOT NULL,
	pier TEXT NOT NULL,
	pile_no TEXT NOT NULL,
	summary TEXT NOT NULL,
	design_depth INTEGER NOT NULL,
	diameter INTEGER NOT NULL,
	layers TEXT NOT NULL,
	rebar TEXT NOT NULL,
	sonic TEXT NOT NULL,
	mud TEXT NOT NULL,
	cleaning TEXT NOT NULL,
	pour TEXT NOT NULL,
	overpour INTEGER NOT NULL,
	line_adjacency TEXT NOT NULL,
	coring TEXT NOT NULL,
	age_period INTEGER NOT NULL,
	max_retries INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS trace (
	pile_id TEXT NOT NULL REFERENCES tasks(id),
	seq INTEGER NOT NULL,
	operation_id TEXT NOT NULL,
	time INTEGER NOT NULL,
	event_type TEXT NOT NULL,
	batch_litres INTEGER NOT NULL,
	total_litres INTEGER NOT NULL,
	theory_level INTEGER NOT NULL,
	measured_level INTEGER NOT NULL,
	conduit_prefix INTEGER NOT NULL,
	embedment INTEGER NOT NULL,
	overpour INTEGER NOT NULL,
	PRIMARY KEY (pile_id, seq)
);
CREATE TABLE IF NOT EXISTS conduits (
	pile_id TEXT PRIMARY KEY REFERENCES tasks(id),
	segments TEXT NOT NULL,
	active_prefix INTEGER NOT NULL,
	bottom_depth INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS batches (
	id TEXT PRIMARY KEY,
	initial INTEGER NOT NULL,
	deducted INTEGER NOT NULL DEFAULT 0,
	CHECK (deducted >= 0 AND deducted <= initial)
);
CREATE TABLE IF NOT EXISTS leases (
	token TEXT PRIMARY KEY,
	device_type TEXT NOT NULL,
	resource_id TEXT NOT NULL,
	holder TEXT NOT NULL,
	start INTEGER NOT NULL,
	end INTEGER NOT NULL,
	status TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS evidence (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	pile_id TEXT NOT NULL REFERENCES tasks(id),
	type TEXT NOT NULL,
	range_start INTEGER NOT NULL,
	range_end INTEGER NOT NULL,
	value INTEGER NOT NULL,
	device_call_id TEXT NOT NULL,
	time INTEGER NOT NULL,
	generation INTEGER NOT NULL,
	digest TEXT NOT NULL,
	valid INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS retries (
	id TEXT PRIMARY KEY,
	pile_id TEXT NOT NULL REFERENCES tasks(id),
	request TEXT NOT NULL,
	attempts INTEGER NOT NULL DEFAULT 0,
	next_retry INTEGER NOT NULL,
	failure_code TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS generations (
	pile_id TEXT NOT NULL REFERENCES tasks(id),
	generation INTEGER NOT NULL,
	line_evidence TEXT NOT NULL,
	anomaly_ranges TEXT NOT NULL,
	reinspect_set TEXT NOT NULL,
	parent INTEGER NOT NULL,
	conclusion TEXT NOT NULL,
	PRIMARY KEY (pile_id, generation)
);
CREATE TABLE IF NOT EXISTS reviewers (
	pile_id TEXT NOT NULL REFERENCES tasks(id),
	reviewer_id TEXT NOT NULL,
	qualified INTEGER NOT NULL,
	approve INTEGER NOT NULL,
	PRIMARY KEY (pile_id, reviewer_id)
);
CREATE TABLE IF NOT EXISTS terminals (
	pile_id TEXT PRIMARY KEY REFERENCES tasks(id),
	type TEXT NOT NULL,
	credential_no TEXT NOT NULL,
	basis TEXT NOT NULL,
	version INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS idempotency (
	operation_id TEXT PRIMARY KEY,
	request_digest TEXT NOT NULL,
	saved_result TEXT NOT NULL
);
INSERT OR IGNORE INTO schema_version (version) VALUES (0);
`,
	// v2: scope idempotency to (pile_id, operation_id) so the same
	// Idempotency-Key reused on a different pile cannot replay another pile's
	// result. Existing rows are backfilled with an empty pile_id so legacy
	// single-pile records still match a same-key retry on that pile.
	`
ALTER TABLE idempotency ADD COLUMN pile_id TEXT NOT NULL DEFAULT '';
DROP INDEX IF EXISTS idempotency_pile_op;
CREATE UNIQUE INDEX IF NOT EXISTS idempotency_pile_op ON idempotency (pile_id, operation_id);
`,
}

// migrate applies any pending migrations inside a transaction.
func (s *SQLStore) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("store: create schema_version: %w", err)
	}
	var current int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	for i := current; i < len(migrations); i++ {
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("store: migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE schema_version SET version = ?`, i+1); err != nil {
			return fmt.Errorf("store: bump schema version: %w", err)
		}
	}
	return tx.Commit()
}
