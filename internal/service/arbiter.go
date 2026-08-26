package service

import (
	"context"

	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/store"
)

// NewGeneration creates a fresh inspection generation, isolating any previous
// generation's evidence and conclusion.
func (s *Service) NewGeneration(ctx context.Context, id domain.PileID) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := tx.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if err := requireStage(task, domain.StageFinished, domain.CodeGenerationConflict); err != nil {
		return err
	}
	parent := task.Generation
	next := parent + 1
	if err := tx.InsertGeneration(ctx, id, domain.ReviewGeneration{
		Generation: next, Parent: parent,
	}); err != nil {
		return err
	}
	task.Generation = next
	task.Stage = domain.StageInspected
	if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
		return err
	}
	return tx.Commit()
}

// Integrity registers acoustic integrity results for the current generation and
// deterministically expands the re-inspection set.
func (s *Service) Integrity(ctx context.Context, id domain.PileID, req domain.IntegrityRequest) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := tx.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if err := requireStage(task, domain.StageInspected, domain.CodeGenerationConflict); err != nil {
		return err
	}
	if req.Generation != task.Generation {
		return domain.NewError(domain.CodeGenerationConflict, "integrity result generation does not match")
	}
	if task.LastTime < task.AgeDeadline {
		return domain.NewError(domain.CodePourInterrupted, "curing age has not been reached")
	}
	design, err := tx.GetDesign(ctx, id)
	if err != nil {
		return err
	}
	reinspect, lines := domain.ExpandAnomalies(design, req.Lines)
	for _, l := range req.Lines {
		anom := int64(0)
		if l.Anomalous {
			anom = 1
		}
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidenceIntegrity, Range: domain.DepthRange{Start: 0, End: design.DesignDepth},
			Value: anom, Time: task.LastTime, Generation: req.Generation, Valid: !l.Anomalous,
		}); err != nil {
			return err
		}
	}
	conclusion := "pass"
	if len(reinspect) > 0 {
		conclusion = "reinspect"
	}
	gen := domain.ReviewGeneration{
		Generation: req.Generation, LineEvidence: lines,
		AnomalyRanges: reinspect, ReinspectSet: reinspect, Conclusion: conclusion,
	}
	if err := tx.UpdateGeneration(ctx, id, gen); err != nil {
		return err
	}
	return tx.Commit()
}

// CoreResult records core-sampling results, resolving the re-inspection set for
// the current generation.
func (s *Service) CoreResult(ctx context.Context, id domain.PileID, req domain.CoreRequest) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := tx.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if err := requireStage(task, domain.StageInspected, domain.CodeGenerationConflict); err != nil {
		return err
	}
	if req.Generation != task.Generation {
		return domain.NewError(domain.CodeGenerationConflict, "core result generation does not match")
	}
	// Acquire the coring rig lease to enforce mutual exclusion.
	if _, err := acquireLease(tx, ctx, id, req.Device, task.LastTime); err != nil {
		return err
	}
	for _, f := range req.Findings {
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidenceCore, Range: f.Range,
			Value: 1, Time: task.LastTime, Generation: req.Generation, Valid: true,
		}); err != nil {
			return err
		}
	}
	if err := tx.UpdateGeneration(ctx, id, domain.ReviewGeneration{
		Generation: req.Generation, ReinspectSet: nil, Conclusion: "closed",
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// Review records one independent dual-review decision.
func (s *Service) Review(ctx context.Context, id domain.PileID, req domain.ReviewRequest) error {
	if !req.Qualified {
		return domain.NewError(domain.CodeDesignMismatch, "reviewer is not qualified")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.GetTask(ctx, id); err != nil {
		return err
	}
	if err := tx.InsertReviewer(ctx, id, domain.ReviewerDecision{
		ReviewerID: req.ReviewerID, Qualification: "qualified", Approve: req.Approve,
	}); err != nil {
		// Duplicate reviewer id violates the primary key.
		return domain.NewError(domain.CodeDesignMismatch, "reviewer has already submitted")
	}
	return tx.Commit()
}

// Terminate competes for the unique terminal decision via the single-writer
// barrier. Acceptance requires evidence closure and two distinct qualified
// approvals.
func (s *Service) Terminate(ctx context.Context, id domain.PileID, req domain.DecisionRequest) (domain.TerminalRecord, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return domain.TerminalRecord{}, err
	}
	defer tx.Rollback()

	if existing, ok, err := tx.GetTerminal(ctx, id); err != nil {
		return domain.TerminalRecord{}, err
	} else if ok {
		return existing, domain.NewError(domain.CodeTerminalAlreadyDecided, "terminal already decided")
	}

	task, err := tx.GetTask(ctx, id)
	if err != nil {
		return domain.TerminalRecord{}, err
	}
	reviewers, err := tx.GetReviewers(ctx, id)
	if err != nil {
		return domain.TerminalRecord{}, err
	}
	qualified := 0
	approvals := 0
	for _, r := range reviewers {
		if r.Qualification != "" {
			qualified++
			if r.Approve {
				approvals++
			}
		}
	}
	if req.Type == domain.TerminalAccept {
		if qualified < 2 || approvals < 2 {
			return domain.TerminalRecord{}, domain.NewError(domain.CodeDesignMismatch, "two qualified approvals are required for acceptance")
		}
	} else if qualified < 1 {
		return domain.TerminalRecord{}, domain.NewError(domain.CodeDesignMismatch, "a qualified reviewer is required")
	}

	rec := domain.TerminalRecord{
		PileID: id, Type: req.Type, CredentialNo: credential(id, req.Type),
		Basis: req.Basis, Version: task.Version,
	}
	if err := tx.InsertTerminal(ctx, rec); err != nil {
		// Single-writer barrier: another transaction won the race.
		if existing, ok, err := tx.GetTerminal(ctx, id); err == nil && ok {
			return existing, domain.NewError(domain.CodeTerminalAlreadyDecided, "terminal already decided")
		}
		return domain.TerminalRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TerminalRecord{}, err
	}
	return rec, nil
}

// Terminal returns the existing terminal record, if any.
func (s *Service) Terminal(ctx context.Context, id domain.PileID) (domain.TerminalRecord, bool, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return domain.TerminalRecord{}, false, err
	}
	defer tx.Rollback()
	return tx.GetTerminal(ctx, id)
}

var _ domain.IntegrityArbiter = (*Service)(nil)
var _ = store.ErrNotFound
