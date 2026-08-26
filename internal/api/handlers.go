package api

import (
	"encoding/json"
	"net/http"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// handleCreate creates a new pile task.
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if !requireComponents(s.svc) {
		writeError(w, domain.NewError("INTERNAL", "service not wired"))
		return
	}
	var req domain.CreateTaskRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	id, err := s.svc.Design.CreateTask(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": string(id)})
}

// handleLock locks the design snapshot.
func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	if !requireComponents(s.svc) {
		writeError(w, domain.NewError("INTERNAL", "service not wired"))
		return
	}
	snap, err := s.svc.Design.Lock(r.Context(), domain.PileID(r.PathValue("id")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// handleBorehole submits borehole checks.
func (s *Server) handleBorehole(w http.ResponseWriter, r *http.Request) {
	var req domain.BoreholeRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if err := s.svc.Trace.Borehole(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
}

// handleCleaning submits cleaning acceptance samples.
func (s *Server) handleCleaning(w http.ResponseWriter, r *http.Request) {
	var req domain.CleaningRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if err := s.svc.Trace.AcceptCleaning(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
}

// handleCages submits rebar and acoustic-tube coverage.
func (s *Server) handleCages(w http.ResponseWriter, r *http.Request) {
	var req domain.CagesRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if err := s.svc.Trace.Cages(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
}

// handleConduits submits the conduit assembly.
func (s *Server) handleConduits(w http.ResponseWriter, r *http.Request) {
	var req domain.ConduitsRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if err := s.svc.Trace.Conduits(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
}

// handleStartPour performs the first pour.
func (s *Server) handleStartPour(w http.ResponseWriter, r *http.Request) {
	var req domain.StartPourRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if req.OperationID == "" {
		req.OperationID = idempotencyKey(r)
	}
	if err := s.svc.Trace.StartPour(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "poured"})
}

// handlePourEntry performs a continuous pour.
func (s *Server) handlePourEntry(w http.ResponseWriter, r *http.Request) {
	var req domain.PourRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if req.OperationID == "" {
		req.OperationID = idempotencyKey(r)
	}
	if err := s.svc.Trace.PourEntry(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "recorded"})
}

// handleLevelReading records a level re-measurement.
func (s *Server) handleLevelReading(w http.ResponseWriter, r *http.Request) {
	var req domain.LevelRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if req.OperationID == "" {
		req.OperationID = idempotencyKey(r)
	}
	if err := s.svc.Trace.LevelReading(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "recorded"})
}

// handleRemoveSegments removes conduit segments.
func (s *Server) handleRemoveSegments(w http.ResponseWriter, r *http.Request) {
	var req domain.RemoveRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if req.OperationID == "" {
		req.OperationID = idempotencyKey(r)
	}
	if err := s.svc.Trace.RemoveSegments(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "removed"})
}

// handleFinishPour finishes the pour.
func (s *Server) handleFinishPour(w http.ResponseWriter, r *http.Request) {
	var req domain.FinishRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if req.OperationID == "" {
		req.OperationID = idempotencyKey(r)
	}
	if err := s.svc.Trace.FinishPour(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "finished"})
}

// handleRetry replays a failed device call.
func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request) {
	var req domain.RetryRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if err := s.svc.Material.Retry(r.Context(), domain.PileID(r.PathValue("id")), r.PathValue("callID"), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "recorded"})
}

// handleLeases returns the pile's leases.
func (s *Server) handleLeases(w http.ResponseWriter, r *http.Request) {
	leases, err := s.svc.Material.Leases(r.Context(), domain.PileID(r.PathValue("id")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, leases)
}

// handleEvidence returns the evidence chain.
func (s *Server) handleEvidence(w http.ResponseWriter, r *http.Request) {
	ev, err := s.svc.Evidence.Evidence(r.Context(), domain.PileID(r.PathValue("id")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// handleIntegrity registers integrity results.
func (s *Server) handleIntegrity(w http.ResponseWriter, r *http.Request) {
	var req domain.IntegrityRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if err := s.svc.Arbiter.Integrity(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "recorded"})
}

// handleNewGeneration creates a new inspection generation.
func (s *Server) handleNewGeneration(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Arbiter.NewGeneration(r.Context(), domain.PileID(r.PathValue("id"))); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "created"})
}

// handleCoreResult records core results.
func (s *Server) handleCoreResult(w http.ResponseWriter, r *http.Request) {
	var req domain.CoreRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if err := s.svc.Arbiter.CoreResult(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "recorded"})
}

// handleReview records an independent review.
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req domain.ReviewRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if err := s.svc.Arbiter.Review(r.Context(), domain.PileID(r.PathValue("id")), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "recorded"})
}

// handleTerminate competes for the terminal decision.
func (s *Server) handleTerminate(w http.ResponseWriter, r *http.Request) {
	var req domain.DecisionRequest
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	rec, err := s.svc.Arbiter.Terminate(r.Context(), domain.PileID(r.PathValue("id")), req)
	if err != nil {
		if de, ok := err.(*domain.Error); ok && de.Code == domain.CodeTerminalAlreadyDecided {
			writeJSON(w, http.StatusConflict, map[string]any{"terminal": rec, "code": string(de.Code)})
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleTask returns the task state.
func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.svc.Design.Task(r.Context(), domain.PileID(r.PathValue("id")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// handleTrace returns the pour trace.
func (s *Server) handleTrace(w http.ResponseWriter, r *http.Request) {
	trace, err := s.svc.Trace.Trace(r.Context(), domain.PileID(r.PathValue("id")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, trace)
}

// handleTerminal returns the terminal record.
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	rec, ok, err := s.svc.Arbiter.Terminal(r.Context(), domain.PileID(r.PathValue("id")))
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, errorBody{Code: "NOT_FOUND", Message: "no terminal decision"})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleCreateBatch registers a concrete batch.
func (s *Server) handleCreateBatch(w http.ResponseWriter, r *http.Request) {
	var req domain.ConcreteBatch
	if err := decode(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "BAD_REQUEST", Message: err.Error()})
		return
	}
	if err := s.svc.Material.CreateBatch(r.Context(), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": req.ID})
}

// handleBatch returns a batch's conservation state.
func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	b, err := s.svc.Material.Batch(r.Context(), r.PathValue("batchID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

var _ = json.Marshal
