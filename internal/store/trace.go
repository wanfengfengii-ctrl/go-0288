package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// NextTraceSeq returns the next append-only sequence number for a pile's trace.
func (t *Tx) NextTraceSeq(ctx context.Context, id domain.PileID) (int64, error) {
	var seq int64
	err := t.tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM trace WHERE pile_id = ?`, id).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("next trace seq: %w", err)
	}
	return seq, nil
}

// InsertTrace appends a trace entry. The (pile_id, seq) primary key and the
// unique (pile_id, operation_id) constraint guard against duplicates.
func (t *Tx) InsertTrace(ctx context.Context, id domain.PileID, e domain.PourTraceEntry) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO trace (pile_id, seq, operation_id, time, event_type, batch_litres, total_litres,
  theory_level, measured_level, conduit_prefix, embedment, overpour)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, e.Seq, e.OperationID, e.Time, e.EventType, e.BatchLitres, e.TotalLitres,
		e.TheoryLevel, e.MeasuredLevel, e.ConduitPrefix, e.Embedment, e.Overpour)
	if err != nil {
		return fmt.Errorf("insert trace: %w", err)
	}
	return nil
}

// GetTrace loads the full trace in sequence order.
func (t *Tx) GetTrace(ctx context.Context, id domain.PileID) ([]domain.PourTraceEntry, error) {
	rows, err := t.tx.QueryContext(ctx, `
SELECT seq, operation_id, time, event_type, batch_litres, total_litres, theory_level,
  measured_level, conduit_prefix, embedment, overpour
FROM trace WHERE pile_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, fmt.Errorf("get trace: %w", err)
	}
	defer rows.Close()
	var out []domain.PourTraceEntry
	for rows.Next() {
		var e domain.PourTraceEntry
		if err := rows.Scan(&e.Seq, &e.OperationID, &e.Time, &e.EventType, &e.BatchLitres,
			&e.TotalLitres, &e.TheoryLevel, &e.MeasuredLevel, &e.ConduitPrefix, &e.Embedment, &e.Overpour); err != nil {
			return nil, fmt.Errorf("scan trace: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LastTrace returns the most recent trace entry for a pile, if any.
func (t *Tx) LastTrace(ctx context.Context, id domain.PileID) (domain.PourTraceEntry, bool, error) {
	var e domain.PourTraceEntry
	err := t.tx.QueryRowContext(ctx, `
SELECT seq, operation_id, time, event_type, batch_litres, total_litres, theory_level,
  measured_level, conduit_prefix, embedment, overpour
FROM trace WHERE pile_id = ? ORDER BY seq DESC LIMIT 1`, id).Scan(
		&e.Seq, &e.OperationID, &e.Time, &e.EventType, &e.BatchLitres, &e.TotalLitres,
		&e.TheoryLevel, &e.MeasuredLevel, &e.ConduitPrefix, &e.Embedment, &e.Overpour)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PourTraceEntry{}, false, nil
	}
	if err != nil {
		return domain.PourTraceEntry{}, false, fmt.Errorf("last trace: %w", err)
	}
	return e, true, nil
}
