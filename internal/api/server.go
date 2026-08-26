// Package api exposes the JSON HTTP surface of the service. It carries the
// stable error-code contract, idempotency handling, transaction boundaries,
// deterministic ordering and the health check described in component 6.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/store"
)

// Server is the HTTP handler holder for the pile-pouring backend.
type Server struct {
	store domain.Store
	svc   domain.Services
}

// NewServer constructs a server bound to the given persistence and service seam.
func NewServer(store domain.Store, svc domain.Services) *Server {
	return &Server{store: store, svc: svc}
}

// Handler builds the route table for the full public API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)

	mux.HandleFunc("POST /v1/piles", s.handleCreate)
	mux.HandleFunc("POST /v1/piles/{id}/lock", s.handleLock)
	mux.HandleFunc("POST /v1/piles/{id}/borehole-checks", s.handleBorehole)
	mux.HandleFunc("POST /v1/piles/{id}/cleaning-acceptance", s.handleCleaning)
	mux.HandleFunc("POST /v1/piles/{id}/cages", s.handleCages)
	mux.HandleFunc("POST /v1/piles/{id}/conduits", s.handleConduits)
	mux.HandleFunc("POST /v1/piles/{id}/pour/start", s.handleStartPour)
	mux.HandleFunc("POST /v1/piles/{id}/pour/entries", s.handlePourEntry)
	mux.HandleFunc("POST /v1/piles/{id}/pour/level-readings", s.handleLevelReading)
	mux.HandleFunc("POST /v1/piles/{id}/pour/remove-segments", s.handleRemoveSegments)
	mux.HandleFunc("POST /v1/piles/{id}/pour/finish", s.handleFinishPour)
	mux.HandleFunc("POST /v1/piles/{id}/device-calls/{callID}/retry", s.handleRetry)
	mux.HandleFunc("GET /v1/piles/{id}/leases", s.handleLeases)
	mux.HandleFunc("GET /v1/piles/{id}/evidence", s.handleEvidence)
	mux.HandleFunc("POST /v1/piles/{id}/integrity-results", s.handleIntegrity)
	mux.HandleFunc("POST /v1/piles/{id}/review-generations", s.handleNewGeneration)
	mux.HandleFunc("POST /v1/piles/{id}/core-results", s.handleCoreResult)
	mux.HandleFunc("POST /v1/piles/{id}/reviews", s.handleReview)
	mux.HandleFunc("POST /v1/piles/{id}/terminal-decisions", s.handleTerminate)
	mux.HandleFunc("GET /v1/piles/{id}", s.handleTask)
	mux.HandleFunc("GET /v1/piles/{id}/trace", s.handleTrace)
	mux.HandleFunc("GET /v1/piles/{id}/terminal", s.handleTerminal)
	mux.HandleFunc("POST /v1/batches", s.handleCreateBatch)
	mux.HandleFunc("GET /v1/batches/{batchID}", s.handleBatch)

	return mux
}

type healthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

// handleHealth reports service and persistence status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{Status: "ok", DB: "ok"}
	status := http.StatusOK
	if s.store == nil {
		resp.Status = "degraded"
		resp.DB = "unavailable"
		status = http.StatusServiceUnavailable
	} else if err := s.store.Ping(r.Context()); err != nil {
		resp.Status = "degraded"
		resp.DB = "unavailable"
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Reasons []string `json:"reasons,omitempty"`
}

// writeError renders a stable JSON error response with a deterministic HTTP
// status derived from the failure code.
func writeError(w http.ResponseWriter, err error) {
	var de *domain.Error
	if errors.As(err, &de) {
		reasons := make([]string, 0, len(de.Reasons))
		for _, r := range de.Reasons {
			reasons = append(reasons, string(r.Code))
		}
		status := http.StatusUnprocessableEntity
		switch de.Code {
		case domain.CodeIdempotencyConflict, domain.CodeTerminalAlreadyDecided:
			status = http.StatusConflict
		}
		writeJSON(w, status, errorBody{Code: string(de.Code), Message: de.Message, Reasons: reasons})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorBody{Code: "NOT_FOUND", Message: "resource not found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, errorBody{Code: "INTERNAL", Message: err.Error()})
}

// idempotencyKey reads the Idempotency-Key header.
func idempotencyKey(r *http.Request) string { return r.Header.Get("Idempotency-Key") }

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func requireComponents(s domain.Services) bool {
	return s.Design != nil && s.Trace != nil && s.Material != nil && s.Arbiter != nil
}

var _ = context.Background
