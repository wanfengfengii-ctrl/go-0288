package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// InsertDesign stores the locked design snapshot for a pile. The pile_id is the
// primary key, so a second lock attempt violates the unique constraint.
func (t *Tx) InsertDesign(ctx context.Context, id domain.PileID, d domain.DesignSnapshot) error {
	layers, _ := json.Marshal(d.Layers)
	rebar, _ := json.Marshal(d.Rebar)
	sonic, _ := json.Marshal(d.Sonic)
	mud, _ := json.Marshal(d.Mud)
	cleaning, _ := json.Marshal(d.Cleaning)
	pour, _ := json.Marshal(d.Pour)
	adj, _ := json.Marshal(d.LineAdjacency)
	coring, _ := json.Marshal(d.Coring)
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO designs (pile_id, generation, pier, pile_no, summary, design_depth, diameter,
  layers, rebar, sonic, mud, cleaning, pour, overpour, line_adjacency, coring, age_period, max_retries)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, d.Generation, d.Pier, d.PileNo, d.Summary, d.DesignDepth, d.Diameter,
		string(layers), string(rebar), string(sonic), string(mud), string(cleaning),
		string(pour), d.Overpour, string(adj), string(coring), d.AgePeriod, d.MaxRetries)
	if err != nil {
		return fmt.Errorf("insert design: %w", err)
	}
	return nil
}

// GetDesign loads the locked design snapshot for a pile.
func (t *Tx) GetDesign(ctx context.Context, id domain.PileID) (domain.DesignSnapshot, error) {
	var d domain.DesignSnapshot
	var layers, rebar, sonic, mud, cleaning, pour, adj, coring string
	err := t.tx.QueryRowContext(ctx, `
SELECT generation, pier, pile_no, summary, design_depth, diameter, layers, rebar, sonic,
  mud, cleaning, pour, overpour, line_adjacency, coring, age_period, max_retries
FROM designs WHERE pile_id = ?`, id).Scan(
		&d.Generation, &d.Pier, &d.PileNo, &d.Summary, &d.DesignDepth, &d.Diameter,
		&layers, &rebar, &sonic, &mud, &cleaning, &pour, &d.Overpour, &adj, &coring, &d.AgePeriod, &d.MaxRetries)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DesignSnapshot{}, ErrNotFound
	}
	if err != nil {
		return domain.DesignSnapshot{}, fmt.Errorf("get design: %w", err)
	}
	_ = json.Unmarshal([]byte(layers), &d.Layers)
	_ = json.Unmarshal([]byte(rebar), &d.Rebar)
	_ = json.Unmarshal([]byte(sonic), &d.Sonic)
	_ = json.Unmarshal([]byte(mud), &d.Mud)
	_ = json.Unmarshal([]byte(cleaning), &d.Cleaning)
	_ = json.Unmarshal([]byte(pour), &d.Pour)
	_ = json.Unmarshal([]byte(adj), &d.LineAdjacency)
	_ = json.Unmarshal([]byte(coring), &d.Coring)
	return d, nil
}
