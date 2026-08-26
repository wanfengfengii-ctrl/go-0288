package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// GetIdempotency loads a previously saved operation result scoped to the given
// pile. Idempotency is keyed on (pile_id, operation_id) so the same
// Idempotency-Key reused on a different pile never replays another pile's
// result.
func (t *Tx) GetIdempotency(ctx context.Context, pileID domain.PileID, operationID string) (domain.IdempotencyRecord, bool, error) {
	var rec domain.IdempotencyRecord
	err := t.tx.QueryRowContext(ctx, `
SELECT pile_id, operation_id, request_digest, saved_result FROM idempotency
WHERE pile_id = ? AND operation_id = ?`, pileID, operationID).Scan(
		&rec.PileID, &rec.OperationID, &rec.RequestDigest, &rec.SavedResult)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return domain.IdempotencyRecord{}, false, fmt.Errorf("get idempotency: %w", err)
	}
	return rec, true, nil
}

// InsertIdempotency saves an operation result. The (pile_id, operation_id)
// uniqueness constraint is a uniqueness violation; the caller compares digests
// to surface an idempotency conflict or return the original result.
func (t *Tx) InsertIdempotency(ctx context.Context, rec domain.IdempotencyRecord) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO idempotency (pile_id, operation_id, request_digest, saved_result) VALUES (?,?,?,?)`,
		rec.PileID, rec.OperationID, rec.RequestDigest, rec.SavedResult)
	if err != nil {
		return fmt.Errorf("insert idempotency: %w", err)
	}
	return nil
}
