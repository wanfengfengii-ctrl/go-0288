package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// InsertGeneration creates an inspection generation.
func (t *Tx) InsertGeneration(ctx context.Context, id domain.PileID, g domain.ReviewGeneration) error {
	lines, _ := json.Marshal(g.LineEvidence)
	anom, _ := json.Marshal(g.AnomalyRanges)
	rein, _ := json.Marshal(g.ReinspectSet)
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO generations (pile_id, generation, line_evidence, anomaly_ranges, reinspect_set, parent, conclusion)
VALUES (?,?,?,?,?,?,?)`,
		id, g.Generation, string(lines), string(anom), string(rein), g.Parent, g.Conclusion)
	if err != nil {
		return fmt.Errorf("insert generation: %w", err)
	}
	return nil
}

// GetGeneration loads one inspection generation.
func (t *Tx) GetGeneration(ctx context.Context, id domain.PileID, gen domain.Generation) (domain.ReviewGeneration, error) {
	var g domain.ReviewGeneration
	var lines, anom, rein string
	err := t.tx.QueryRowContext(ctx, `
SELECT generation, line_evidence, anomaly_ranges, reinspect_set, parent, conclusion
FROM generations WHERE pile_id = ? AND generation = ?`, id, gen).Scan(
		&g.Generation, &lines, &anom, &rein, &g.Parent, &g.Conclusion)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ReviewGeneration{}, ErrNotFound
	}
	if err != nil {
		return domain.ReviewGeneration{}, fmt.Errorf("get generation: %w", err)
	}
	_ = json.Unmarshal([]byte(lines), &g.LineEvidence)
	_ = json.Unmarshal([]byte(anom), &g.AnomalyRanges)
	_ = json.Unmarshal([]byte(rein), &g.ReinspectSet)
	return g, nil
}

// InsertReviewer records one independent dual-review decision.
func (t *Tx) InsertReviewer(ctx context.Context, id domain.PileID, d domain.ReviewerDecision) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO reviewers (pile_id, reviewer_id, qualified, approve) VALUES (?,?,?,?)`,
		id, d.ReviewerID, boolToInt(d.Qualification != ""), boolToInt(d.Approve))
	if err != nil {
		return fmt.Errorf("insert reviewer: %w", err)
	}
	return nil
}

// GetReviewers returns all reviewer decisions for a pile.
func (t *Tx) GetReviewers(ctx context.Context, id domain.PileID) ([]domain.ReviewerDecision, error) {
	rows, err := t.tx.QueryContext(ctx, `
SELECT reviewer_id, qualified, approve FROM reviewers WHERE pile_id = ? ORDER BY reviewer_id`, id)
	if err != nil {
		return nil, fmt.Errorf("get reviewers: %w", err)
	}
	defer rows.Close()
	var out []domain.ReviewerDecision
	for rows.Next() {
		var d domain.ReviewerDecision
		var qualified, approve int
		if err := rows.Scan(&d.ReviewerID, &qualified, &approve); err != nil {
			return nil, fmt.Errorf("scan reviewer: %w", err)
		}
		d.Approve = approve != 0
		if qualified != 0 {
			d.Qualification = "qualified"
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// InsertTerminal writes the unique terminal record. The pile_id primary key is
// the single-writer barrier: a second insert for the same pile violates the
// unique constraint and the caller surfaces the existing terminal instead.
func (t *Tx) InsertTerminal(ctx context.Context, rec domain.TerminalRecord) error {
	_, err := t.tx.ExecContext(ctx, `
INSERT INTO terminals (pile_id, type, credential_no, basis, version) VALUES (?,?,?,?,?)`,
		rec.PileID, rec.Type, rec.CredentialNo, rec.Basis, rec.Version)
	if err != nil {
		return fmt.Errorf("insert terminal: %w", err)
	}
	return nil
}

// GetTerminal loads the terminal record for a pile, if any.
func (t *Tx) GetTerminal(ctx context.Context, id domain.PileID) (domain.TerminalRecord, bool, error) {
	var rec domain.TerminalRecord
	err := t.tx.QueryRowContext(ctx, `
SELECT pile_id, type, credential_no, basis, version FROM terminals WHERE pile_id = ?`, id).Scan(
		&rec.PileID, &rec.Type, &rec.CredentialNo, &rec.Basis, &rec.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TerminalRecord{}, false, nil
	}
	if err != nil {
		return domain.TerminalRecord{}, false, fmt.Errorf("get terminal: %w", err)
	}
	return rec, true, nil
}

// UpdateGeneration overwrites a generation's derived fields (evidence lines,
// anomaly ranges, re-inspection set and conclusion).
func (t *Tx) UpdateGeneration(ctx context.Context, id domain.PileID, g domain.ReviewGeneration) error {
	lines, _ := json.Marshal(g.LineEvidence)
	anom, _ := json.Marshal(g.AnomalyRanges)
	rein, _ := json.Marshal(g.ReinspectSet)
	_, err := t.tx.ExecContext(ctx, `
UPDATE generations SET line_evidence=?, anomaly_ranges=?, reinspect_set=?, conclusion=?
WHERE pile_id=? AND generation=?`,
		string(lines), string(anom), string(rein), g.Conclusion, id, g.Generation)
	if err != nil {
		return fmt.Errorf("update generation: %w", err)
	}
	return nil
}

// LatestGeneration returns the highest generation number for a pile.
func (t *Tx) LatestGeneration(ctx context.Context, id domain.PileID) (domain.Generation, error) {
	var g domain.Generation
	err := t.tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(generation),0) FROM generations WHERE pile_id = ?`, id).Scan(&g)
	if err != nil {
		return 0, fmt.Errorf("latest generation: %w", err)
	}
	return g, nil
}
