package service

import (
	"context"
	"strings"

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
		// Device recovered. Complete the deferred operation the retry stood in
		// for, mirroring that operation's success path so the trace and last
		// logical time advance exactly as if the device had answered on the
		// first attempt (failure boundary 7: no state advance on failure; the
		// success path must therefore advance on recovery).
		switch rc.Request {
		case "sounding-line":
			task, err := tx.GetTask(ctx, id)
			if err != nil {
				return err
			}
			conduit, err := tx.GetConduit(ctx, id)
			if err != nil {
				return err
			}
			prev, ok, err := tx.LastTrace(ctx, id)
			if err != nil {
				return err
			}
			if !ok {
				return domain.NewError(domain.CodePourInterrupted, "pour has not started")
			}
			if req.Time <= prev.Time {
				return domain.NewError(domain.CodePourInterrupted, "logical time must strictly increase")
			}
			seq, err := tx.NextTraceSeq(ctx, id)
			if err != nil {
				return err
			}
			entry := domain.PourTraceEntry{
				Seq: seq, OperationID: strings.TrimSuffix(callID, "-call"), Time: req.Time,
				EventType: domain.PourLevelReading, TotalLitres: prev.TotalLitres,
				TheoryLevel: prev.TheoryLevel, MeasuredLevel: req.Reading,
				ConduitPrefix: conduit.ActivePrefix, Embedment: conduit.BottomDepth - prev.TheoryLevel,
				Overpour: prev.Overpour,
			}
			if err := tx.InsertTrace(ctx, id, entry); err != nil {
				return err
			}
			task.LastTime = req.Time
			if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
				return err
			}
		default:
			// Non-sounding retries (e.g. late core receipts) archive the
			// reading as valid evidence without advancing the pour trace.
			if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
				Type: domain.EvidenceCore, Range: domain.DepthRange{Start: 0, End: 0},
				Value: req.Reading, DeviceCallID: callID, Time: req.Time, Generation: 1, Valid: true,
			}); err != nil {
				return err
			}
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
