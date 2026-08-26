// Package service implements the business components of the pile-pouring
// quality-closure backend on top of the SQLite store. Each exported method maps
// to a named business flow, domain rule or failure boundary in PROJECT_SPEC and
// is reachable from the HTTP API.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/store"
)

// Service implements all five component interfaces (DesignCatalog, TaskTrace,
// MaterialLease, EvidenceRecorder, IntegrityArbiter) over the persistence
// boundary.
type Service struct {
	store *store.SQLStore
}

// New constructs a Service backed by the given store.
func New(s *store.SQLStore) *Service {
	return &Service{store: s}
}

// Store returns the underlying persistence boundary (used by the entry point
// for the health endpoint).
func (s *Service) Store() domain.Store { return s.store }

// begin starts a transaction, deferring rollback to the caller.
func (s *Service) begin(ctx context.Context) (*store.Tx, error) {
	return s.store.Begin(ctx)
}

// digest computes a canonical SHA-256 digest of a request for idempotency.
func digest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// idemResult runs a transactional mutation with optional idempotency. When
// operationID is non-empty it guarantees at-most-once execution: a matching
// prior digest returns the original result, a differing digest returns
// IDEMPOTENCY_CONFLICT.
func (s *Service) idemResult(ctx context.Context, operationID string, req any, fn func(tx *store.Tx) (string, error)) (string, error) {
	d := digest(req)
	tx, err := s.begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if operationID != "" {
		rec, found, err := tx.GetIdempotency(ctx, operationID)
		if err != nil {
			return "", err
		}
		if found {
			if rec.RequestDigest != d {
				return "", domain.NewError(domain.CodeIdempotencyConflict, "same operation id with different content")
			}
			return rec.SavedResult, nil
		}
	}

	result, err := fn(tx)
	if err != nil {
		return "", err
	}
	if operationID != "" {
		if err := tx.InsertIdempotency(ctx, domain.IdempotencyRecord{
			OperationID: operationID, RequestDigest: d, SavedResult: result,
		}); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return result, nil
}

// newPileID generates a non-colliding pile identifier.
func newPileID() domain.PileID {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return domain.PileID("pile-" + hex.EncodeToString(b))
}

// newToken generates a non-colliding lease token.
func newToken() domain.LeaseToken {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return domain.LeaseToken("tok-" + hex.EncodeToString(b))
}

// credential renders a deterministic, non-repeating terminal credential.
func credential(id domain.PileID, t domain.TerminalType) string {
	h := sha256.Sum256([]byte(string(id) + ":" + string(t)))
	return "TERM-" + hex.EncodeToString(h[:12])
}
