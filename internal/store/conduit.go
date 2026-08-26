package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// InsertConduit stores the validated conduit assembly for a pile.
func (t *Tx) InsertConduit(ctx context.Context, id domain.PileID, c domain.ConduitAssembly) error {
	segments, _ := json.Marshal(c.Segments)
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO conduits (pile_id, segments, active_prefix, bottom_depth) VALUES (?,?,?,?)`,
		id, string(segments), c.ActivePrefix, c.BottomDepth)
	if err != nil {
		return fmt.Errorf("insert conduit: %w", err)
	}
	return nil
}

// GetConduit loads the conduit assembly for a pile.
func (t *Tx) GetConduit(ctx context.Context, id domain.PileID) (domain.ConduitAssembly, error) {
	var c domain.ConduitAssembly
	var segments string
	err := t.tx.QueryRowContext(ctx, `
SELECT segments, active_prefix, bottom_depth FROM conduits WHERE pile_id = ?`, id).Scan(
		&segments, &c.ActivePrefix, &c.BottomDepth)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ConduitAssembly{}, ErrNotFound
	}
	if err != nil {
		return domain.ConduitAssembly{}, fmt.Errorf("get conduit: %w", err)
	}
	_ = json.Unmarshal([]byte(segments), &c.Segments)
	return c, nil
}

// UpdateConduit mutates the active prefix and bottom depth after a removal.
func (t *Tx) UpdateConduit(ctx context.Context, id domain.PileID, c domain.ConduitAssembly) error {
	_, err := t.tx.ExecContext(ctx, `
UPDATE conduits SET active_prefix=?, bottom_depth=? WHERE pile_id=?`,
		c.ActivePrefix, c.BottomDepth, id)
	if err != nil {
		return fmt.Errorf("update conduit: %w", err)
	}
	return nil
}
