package service

import (
	"context"
	"errors"

	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/store"
)

// mapDeviceOutcome maps a scripted device outcome to a stable failure code.
// An empty outcome or "success" means the device returned a valid reading.
func mapDeviceOutcome(outcome string) (domain.ErrorCode, bool) {
	switch outcome {
	case "rejected":
		return domain.CodeDeviceRejected, true
	case "timeout":
		return domain.CodeDeviceTimeout, true
	case "malformed":
		return domain.CodeDeviceRejected, true
	default:
		return "", false
	}
}

// failDevice records a pending retry call in its own committed transaction and
// returns the mapped device error. The operation's business state is never
// advanced on a device failure (failure boundary 7). Replaying the same
// timed-out operation with the same Idempotency-Key is stable: if a pending
// retry call already exists for this operation it is preserved as-is, so the
// client observes the same device error and a single awaitable retry.
func (s *Service) failDevice(ctx context.Context, id domain.PileID, operationID, request, outcome string, now domain.LogicalTime) error {
	code, failed := mapDeviceOutcome(outcome)
	if !failed {
		return nil
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	callID := retryCallID(operationID)
	if _, err := tx.GetRetry(ctx, id, callID); err == nil {
		// A pending retry for this operation already exists; keep it and return
		// the same device failure rather than colliding on the unique key.
		return domain.NewError(code, "device "+outcome)
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err := tx.InsertRetry(ctx, id, domain.RetryCall{
		ID: callID, Request: request, Attempts: 1, NextRetry: now + 1, FailureCode: code,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return domain.NewError(code, "device "+outcome)
}

// RetryCallID renders the deterministic retry-call identifier for an operation.
func retryCallID(operationID string) string { return operationID + "-call" }

var _ = store.ErrNotFound
