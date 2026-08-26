package service

import (
	"context"

	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/store"
)

// CreateBatch registers a concrete batch pool.
func (s *Service) CreateBatch(ctx context.Context, batch domain.ConcreteBatch) error {
	if batch.ID == "" || batch.Initial < 0 {
		return domain.NewError(domain.CodeConcreteInsufficient, "invalid batch")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := tx.InsertBatch(ctx, batch); err != nil {
		return err
	}
	return tx.Commit()
}

// Batch returns a batch's conservation state.
func (s *Service) Batch(ctx context.Context, batchID string) (domain.ConcreteBatch, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return domain.ConcreteBatch{}, err
	}
	defer tx.Rollback()
	return tx.GetBatch(ctx, batchID)
}

// Leases returns the leases held by a pile.
func (s *Service) Leases(ctx context.Context, id domain.PileID) ([]domain.DeviceLease, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return tx.GetLeases(ctx, id)
}

// Retry replays a failed device call with a scripted outcome. On success the
// reading is archived and the retry resolved; on failure the attempt counter and
// next-retry time advance deterministically and, past the locked retry limit, a
// quality anomaly is recorded.
func (s *Service) Retry(ctx context.Context, id domain.PileID, callID string, req domain.RetryRequest) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rc, err := tx.GetRetry(ctx, id, callID)
	if err != nil {
		if err == store.ErrNotFound {
			return domain.NewError(domain.CodeDeviceRejected, "retry call not found")
		}
		return err
	}

	if code, failed := mapDeviceOutcome(req.Outcome); !failed {
		// Device recovered: archive the reading as valid evidence and resolve.
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidenceCore, Range: domain.DepthRange{Start: 0, End: 0},
			Value: req.Reading, DeviceCallID: callID, Time: req.Time, Generation: 1, Valid: true,
		}); err != nil {
			return err
		}
		if err := tx.DeleteRetry(ctx, callID); err != nil {
			return err
		}
		return tx.Commit()
	} else {
		rc.Attempts++
		rc.FailureCode = code
		rc.NextRetry = req.Time + 1
		if err := tx.UpdateRetry(ctx, rc); err != nil {
			return err
		}
		design, err := tx.GetDesign(ctx, id)
		if err != nil {
			return err
		}
		if rc.Attempts > design.MaxRetries {
			if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
				Type: domain.EvidenceCore, Range: domain.DepthRange{Start: 0, End: 0},
				Value: int64(rc.Attempts), DeviceCallID: callID, Time: req.Time,
				Generation: 1, Valid: false,
			}); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
}

var _ domain.MaterialLease = (*Service)(nil)
