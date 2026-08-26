package service

import (
	"context"

	"deep-pile-pour-integrity-closure/internal/domain"
)

// CreateTask validates and persists a new unlocked pile task plus its design
// baseline (design generation 1), returning the pile identifier.
func (s *Service) CreateTask(ctx context.Context, req domain.CreateTaskRequest) (domain.PileID, error) {
	if err := req.Validate(); err != nil {
		return "", err
	}
	id := newPileID()
	tx, err := s.begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	task := domain.PileTask{ID: id, Stage: domain.StageCreated}
	if err := tx.InsertTask(ctx, task); err != nil {
		return "", err
	}
	if err := tx.InsertDesign(ctx, id, req.Snapshot(1)); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// Lock validates and freezes the design snapshot, advancing the task to the
// locked stage. A second lock or a lock of a non-created task is rejected with
// DESIGN_MISMATCH.
func (s *Service) Lock(ctx context.Context, id domain.PileID) (domain.DesignSnapshot, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return domain.DesignSnapshot{}, err
	}
	defer tx.Rollback()

	task, err := tx.GetTask(ctx, id)
	if err != nil {
		return domain.DesignSnapshot{}, err
	}
	if task.Stage != domain.StageCreated {
		return domain.DesignSnapshot{}, domain.NewError(domain.CodeDesignMismatch, "task is already locked")
	}
	design, err := tx.GetDesign(ctx, id)
	if err != nil {
		return domain.DesignSnapshot{}, err
	}
	if err := domain.ValidateSnapshot(design); err != nil {
		return domain.DesignSnapshot{}, err
	}
	task.Stage = domain.StageLocked
	task.LockedVersion = 1
	task.Generation = 1
	if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
		return domain.DesignSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.DesignSnapshot{}, err
	}
	return design, nil
}

// Snapshot returns the locked design snapshot.
func (s *Service) Snapshot(ctx context.Context, id domain.PileID) (domain.DesignSnapshot, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return domain.DesignSnapshot{}, err
	}
	defer tx.Rollback()
	return tx.GetDesign(ctx, id)
}

// Task returns the current task state.
func (s *Service) Task(ctx context.Context, id domain.PileID) (domain.PileTask, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return domain.PileTask{}, err
	}
	defer tx.Rollback()
	return tx.GetTask(ctx, id)
}

var _ domain.DesignCatalog = (*Service)(nil)
