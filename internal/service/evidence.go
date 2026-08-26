package service

import (
	"context"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// Evidence returns the immutable evidence chain in deterministic order.
func (s *Service) Evidence(ctx context.Context, id domain.PileID) ([]domain.InspectionEvidence, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return tx.GetEvidence(ctx, id)
}

var _ domain.EvidenceRecorder = (*Service)(nil)
