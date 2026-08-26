package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// GetIdempotency loads a previously saved operation result.
func (t *Tx) GetIdempotency(ctx context.Context, operationID string) (domain.IdempotencyRecord, bool, error) {
	var rec domain.IdempotencyRecord
	err := t.tx.QueryRowContext(ctx, `
SELECT operation_id, request_digest, saved_result FROM idempotency WHERE operation_id = ?`, operationID).Scan(
		&rec.OperationID, &rec.RequestDigest, &rec.SavedResult)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return domain.IdempotencyRecord{}, false, fmt.Errorf("get idempotency: %w", err)
	}
	return rec, true, nil
}

// InsertIdempotency saves an operation result. A duplicate operation identifier
// is a uniqueness violation; the caller compares digests to surface an
// idempotency conflict or return the original result.
func (t *Tx) InsertIdempotency(ctx context.Context, rec domain.IdempotencyRecord) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO idempotency (operation_id, request_digest, saved_result) VALUES (?,?,?)`,
		rec.OperationID, rec.RequestDigest, rec.SavedResult)
	if err != nil {
		return fmt.Errorf("insert idempotency: %w", err)
	}
	return nil
}
