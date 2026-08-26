package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// ErrNotFound reports that an entity does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrVersionConflict reports an optimistic version mismatch.
var ErrVersionConflict = errors.New("store: version conflict")

// InsertTask creates a new pile task. A duplicate identifier is a uniqueness
// violation.
func (t *Tx) InsertTask(ctx context.Context, task domain.PileTask) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO tasks (id, stage, locked_version, generation, last_time, age_deadline, version, terminal)
VALUES (?,?,?,?,?,?,?,?)`,
		task.ID, task.Stage, task.LockedVersion, task.Generation, task.LastTime,
		task.AgeDeadline, task.Version, task.Terminal)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	return nil
}

// GetTask loads a task by identifier.
func (t *Tx) GetTask(ctx context.Context, id domain.PileID) (domain.PileTask, error) {
	var task domain.PileTask
	err := t.tx.QueryRowContext(ctx, `
SELECT id, stage, locked_version, generation, last_time, age_deadline, version, terminal
FROM tasks WHERE id = ?`, id).Scan(
		&task.ID, &task.Stage, &task.LockedVersion, &task.Generation,
		&task.LastTime, &task.AgeDeadline, &task.Version, &task.Terminal)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PileTask{}, ErrNotFound
	}
	if err != nil {
		return domain.PileTask{}, fmt.Errorf("get task: %w", err)
	}
	return task, nil
}

// UpdateTask writes a task back using optimistic version checking: the update
// only succeeds when the stored version matches expectedVersion, after which the
// version is incremented.
func (t *Tx) UpdateTask(ctx context.Context, task domain.PileTask, expectedVersion int64) error {
	res, err := t.tx.ExecContext(ctx, `
UPDATE tasks SET stage=?, locked_version=?, generation=?, last_time=?, age_deadline=?, version=version+1, terminal=?
WHERE id=? AND version=?`,
		task.Stage, task.LockedVersion, task.Generation, task.LastTime,
		task.AgeDeadline, task.Terminal, task.ID, expectedVersion)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update task rows: %w", err)
	}
	if n == 0 {
		return ErrVersionConflict
	}
	return nil
}
