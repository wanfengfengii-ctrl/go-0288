package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// InsertBatch registers a concrete batch.
func (t *Tx) InsertBatch(ctx context.Context, b domain.ConcreteBatch) error {
	_, err := t.tx.ExecContext(ctx, `INSERT INTO batches (id, initial, deducted) VALUES (?,?,?)`,
		b.ID, b.Initial, b.Deducted)
	if err != nil {
		return fmt.Errorf("insert batch: %w", err)
	}
	return nil
}

// GetBatch loads a batch's conservation state.
func (t *Tx) GetBatch(ctx context.Context, id string) (domain.ConcreteBatch, error) {
	var b domain.ConcreteBatch
	err := t.tx.QueryRowContext(ctx, `SELECT id, initial, deducted FROM batches WHERE id = ?`, id).Scan(
		&b.ID, &b.Initial, &b.Deducted)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ConcreteBatch{}, ErrNotFound
	}
	if err != nil {
		return domain.ConcreteBatch{}, fmt.Errorf("get batch: %w", err)
	}
	return b, nil
}

// ReserveBatch atomically deducts litres using an optimistic guard on the
// previously observed deducted value, so a concurrent over-reservation leaves no
// partial state and the CHECK constraint enforces non-negative balance.
func (t *Tx) ReserveBatch(ctx context.Context, id string, litres int64, expectedDeducted int64) error {
	res, err := t.tx.ExecContext(ctx,
		`UPDATE batches SET deducted = deducted + ? WHERE id = ? AND deducted = ?`, litres, id, expectedDeducted)
	if err != nil {
		return fmt.Errorf("reserve batch: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reserve batch rows: %w", err)
	}
	if n == 0 {
		return ErrVersionConflict
	}
	return nil
}

// AcquireLease inserts a device lease. A duplicate token is a uniqueness
// violation; a conflicting active lease on the same resource is detected by the
// caller before insertion.
func (t *Tx) AcquireLease(ctx context.Context, l domain.DeviceLease) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO leases (token, device_type, resource_id, holder, start, end, status)
VALUES (?,?,?,?,?,?,?)`,
		l.Token, l.DeviceType, l.ResourceID, l.Holder, l.Start, l.End, l.Status)
	if err != nil {
		return fmt.Errorf("acquire lease: %w", err)
	}
	return nil
}

// GetLeases returns the leases held by a pile.
func (t *Tx) GetLeases(ctx context.Context, id domain.PileID) ([]domain.DeviceLease, error) {
	rows, err := t.tx.QueryContext(ctx, `
SELECT token, device_type, resource_id, holder, start, end, status
FROM leases WHERE holder = ? ORDER BY device_type, resource_id, start`, id)
	if err != nil {
		return nil, fmt.Errorf("get leases: %w", err)
	}
	defer rows.Close()
	var out []domain.DeviceLease
	for rows.Next() {
		var l domain.DeviceLease
		if err := rows.Scan(&l.Token, &l.DeviceType, &l.ResourceID, &l.Holder, &l.Start, &l.End, &l.Status); err != nil {
			return nil, fmt.Errorf("scan lease: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// FindActiveLease locates an active lease on a device resource at the given
// logical time, if one exists.
func (t *Tx) FindActiveLease(ctx context.Context, deviceType domain.DeviceType, resourceID string, now domain.LogicalTime) (domain.DeviceLease, bool, error) {
	var l domain.DeviceLease
	err := t.tx.QueryRowContext(ctx, `
SELECT token, device_type, resource_id, holder, start, end, status
FROM leases WHERE device_type = ? AND resource_id = ? AND status = 'active' AND end > ?
LIMIT 1`, deviceType, resourceID, now).Scan(
		&l.Token, &l.DeviceType, &l.ResourceID, &l.Holder, &l.Start, &l.End, &l.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DeviceLease{}, false, nil
	}
	if err != nil {
		return domain.DeviceLease{}, false, fmt.Errorf("find active lease: %w", err)
	}
	return l, true, nil
}

// ReleaseLease marks a lease as released by its token.
func (t *Tx) ReleaseLease(ctx context.Context, token domain.LeaseToken) error {
	_, err := t.tx.ExecContext(ctx, `UPDATE leases SET status = 'released' WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("release lease: %w", err)
	}
	return nil
}

// ExpireLeases marks all active leases whose end time is not after now as
// expired, returning the number affected. This is the only path by which a lease
// transitions to expired.
func (t *Tx) ExpireLeases(ctx context.Context, now domain.LogicalTime) (int64, error) {
	res, err := t.tx.ExecContext(ctx, `UPDATE leases SET status = 'expired' WHERE status = 'active' AND end <= ?`, now)
	if err != nil {
		return 0, fmt.Errorf("expire leases: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
