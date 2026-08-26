package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// InsertEvidence records one immutable evidence item.
func (t *Tx) InsertEvidence(ctx context.Context, id domain.PileID, ev domain.InspectionEvidence) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO evidence (pile_id, type, range_start, range_end, value, device_call_id, time, generation, digest, valid)
VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, ev.Type, ev.Range.Start, ev.Range.End, ev.Value, ev.DeviceCallID, ev.Time,
		ev.Generation, ev.PayloadDigest, boolToInt(ev.Valid))
	if err != nil {
		return fmt.Errorf("insert evidence: %w", err)
	}
	return nil
}

// GetEvidence returns the evidence chain in deterministic order (pier, pile,
// depth interval, logical time, conduit segment, line).
func (t *Tx) GetEvidence(ctx context.Context, id domain.PileID) ([]domain.InspectionEvidence, error) {
	rows, err := t.tx.QueryContext(ctx, `
SELECT type, range_start, range_end, value, device_call_id, time, generation, digest, valid
FROM evidence WHERE pile_id = ? ORDER BY type, range_start, range_end, time, device_call_id`, id)
	if err != nil {
		return nil, fmt.Errorf("get evidence: %w", err)
	}
	defer rows.Close()
	var out []domain.InspectionEvidence
	for rows.Next() {
		var ev domain.InspectionEvidence
		var valid int
		if err := rows.Scan(&ev.Type, &ev.Range.Start, &ev.Range.End, &ev.Value,
			&ev.DeviceCallID, &ev.Time, &ev.Generation, &ev.PayloadDigest, &valid); err != nil {
			return nil, fmt.Errorf("scan evidence: %w", err)
		}
		ev.Valid = valid != 0
		out = append(out, ev)
	}
	return out, rows.Err()
}

// InsertRetry records a pending device retry call.
func (t *Tx) InsertRetry(ctx context.Context, id domain.PileID, rc domain.RetryCall) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO retries (id, pile_id, request, attempts, next_retry, failure_code)
VALUES (?,?,?,?,?,?)`,
		rc.ID, id, rc.Request, rc.Attempts, rc.NextRetry, rc.FailureCode)
	if err != nil {
		return fmt.Errorf("insert retry: %w", err)
	}
	return nil
}

// GetRetry loads a retry call by identifier, verifying it belongs to the given
// pile so late receipts for other tasks are rejected.
func (t *Tx) GetRetry(ctx context.Context, pileID domain.PileID, id string) (domain.RetryCall, error) {
	var rc domain.RetryCall
	err := t.tx.QueryRowContext(ctx, `
SELECT id, request, attempts, next_retry, failure_code FROM retries WHERE id = ? AND pile_id = ?`, id, pileID).Scan(
		&rc.ID, &rc.Request, &rc.Attempts, &rc.NextRetry, &rc.FailureCode)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RetryCall{}, ErrNotFound
	}
	if err != nil {
		return domain.RetryCall{}, fmt.Errorf("get retry: %w", err)
	}
	return rc, nil
}

// UpdateRetry mutates a retry call's attempt count, next retry time and failure
// code.
func (t *Tx) UpdateRetry(ctx context.Context, rc domain.RetryCall) error {
	_, err := t.tx.ExecContext(ctx, `
UPDATE retries SET attempts=?, next_retry=?, failure_code=? WHERE id=?`,
		rc.Attempts, rc.NextRetry, rc.FailureCode, rc.ID)
	if err != nil {
		return fmt.Errorf("update retry: %w", err)
	}
	return nil
}

// DeleteRetry removes a satisfied retry call.
func (t *Tx) DeleteRetry(ctx context.Context, id string) error {
	_, err := t.tx.ExecContext(ctx, `DELETE FROM retries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete retry: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
