package service

import (
	"context"

	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/fixed"
	"deep-pile-pour-integrity-closure/internal/store"
)

// requireStage asserts a task's current stage, returning a stable code on
// mismatch.
func requireStage(task domain.PileTask, want domain.Stage, code domain.ErrorCode) error {
	if task.Stage != want {
		return domain.NewError(code, "unexpected stage "+string(task.Stage))
	}
	return nil
}

// sampleMap indexes point samples by depth.
func sampleMap(samples []domain.PointSample) map[int64]int64 {
	m := make(map[int64]int64, len(samples))
	for _, s := range samples {
		m[s.Depth] = s.Value
	}
	return m
}

// mudMap indexes mud samples by depth.
func mudMap(samples []domain.MudSample) map[int64]domain.MudSample {
	m := make(map[int64]domain.MudSample, len(samples))
	for _, s := range samples {
		m[s.Depth] = s
	}
	return m
}

// Borehole validates and records final-depth, aperture and sediment samples,
// then advances the task to the borehole-checked stage.
func (s *Service) Borehole(ctx context.Context, id domain.PileID, req domain.BoreholeRequest) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := tx.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if err := requireStage(task, domain.StageLocked, domain.CodeBoreholeConflict); err != nil {
		return err
	}
	design, err := tx.GetDesign(ctx, id)
	if err != nil {
		return err
	}
	if req.FinalDepth != design.DesignDepth {
		return domain.NewError(domain.CodeBoreholeConflict, "final depth does not match the design depth")
	}

	points := domain.CheckPoints(design.Layers, design.DesignDepth)
	aperture := sampleMap(req.Aperture)
	sediment := sampleMap(req.Sediment)

	var reasons []domain.Reason
	for _, p := range points {
		v, ok := aperture[p]
		if !ok {
			reasons = append(reasons, domain.Reason{DepthStart: p, DepthEnd: p, Code: domain.CodeSampleMissing, Detail: "missing aperture sample"})
			continue
		}
		lo := design.Diameter - design.Cleaning.ApertureTolerance
		hi := design.Diameter + design.Cleaning.ApertureTolerance
		if v < lo || v > hi {
			reasons = append(reasons, domain.Reason{DepthStart: p, DepthEnd: p, Code: domain.CodeBoreholeConflict, Detail: "aperture out of tolerance"})
		}
	}
	for _, p := range points {
		v, ok := sediment[p]
		if !ok {
			reasons = append(reasons, domain.Reason{DepthStart: p, DepthEnd: p, Code: domain.CodeSampleMissing, Detail: "missing sediment sample"})
			continue
		}
		if v > design.Cleaning.SedimentMax {
			reasons = append(reasons, domain.Reason{DepthStart: p, DepthEnd: p, Code: domain.CodeBoreholeConflict, Detail: "sediment thickness exceeds limit"})
		}
	}
	if len(reasons) > 0 {
		return (&domain.Error{Code: domain.CodeBoreholeConflict, Message: "borehole checks failed", Reasons: reasons}).Normalize()
	}

	for _, s := range req.Aperture {
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidenceBorehole, Range: domain.DepthRange{Start: s.Depth, End: s.Depth},
			Value: s.Value, Time: task.LastTime, Generation: 1, Valid: true,
		}); err != nil {
			return err
		}
	}
	for _, s := range req.Sediment {
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidenceBorehole, Range: domain.DepthRange{Start: s.Depth, End: s.Depth},
			Value: s.Value, Time: task.LastTime, Generation: 1, Valid: true,
		}); err != nil {
			return err
		}
	}
	task.Stage = domain.StageBoreholeChecked
	if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
		return err
	}
	return tx.Commit()
}

// AcceptCleaning validates and records slurry samples, advancing to the
// cleaning-accepted stage.
func (s *Service) AcceptCleaning(ctx context.Context, id domain.PileID, req domain.CleaningRequest) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := tx.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if err := requireStage(task, domain.StageBoreholeChecked, domain.CodeSampleMissing); err != nil {
		return err
	}
	design, err := tx.GetDesign(ctx, id)
	if err != nil {
		return err
	}
	points := domain.CheckPoints(design.Layers, design.DesignDepth)
	mud := mudMap(req.Mud)

	var reasons []domain.Reason
	for _, p := range points {
		m, ok := mud[p]
		if !ok {
			reasons = append(reasons, domain.Reason{DepthStart: p, DepthEnd: p, Code: domain.CodeSampleMissing, Detail: "missing mud sample"})
			continue
		}
		if m.SpecificGravity < design.Mud.SpecificGravityMin || m.SpecificGravity > design.Mud.SpecificGravityMax {
			reasons = append(reasons, domain.Reason{DepthStart: p, DepthEnd: p, Code: domain.CodeBoreholeConflict, Detail: "specific gravity out of range"})
		}
		if m.Viscosity < design.Mud.ViscosityMin || m.Viscosity > design.Mud.ViscosityMax {
			reasons = append(reasons, domain.Reason{DepthStart: p, DepthEnd: p, Code: domain.CodeBoreholeConflict, Detail: "viscosity out of range"})
		}
		if m.SandContent > design.Mud.SandContentMax {
			reasons = append(reasons, domain.Reason{DepthStart: p, DepthEnd: p, Code: domain.CodeBoreholeConflict, Detail: "sand content exceeds limit"})
		}
	}
	if len(reasons) > 0 {
		return (&domain.Error{Code: domain.CodeSampleMissing, Message: "cleaning acceptance failed", Reasons: reasons}).Normalize()
	}
	for _, m := range req.Mud {
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidenceMud, Range: domain.DepthRange{Start: m.Depth, End: m.Depth},
			Value: m.SpecificGravity, Time: task.LastTime, Generation: 1, Valid: true,
		}); err != nil {
			return err
		}
	}
	task.Stage = domain.StageCleaningAccepted
	if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
		return err
	}
	return tx.Commit()
}

// Cages validates and records rebar and acoustic-tube coverage.
func (s *Service) Cages(ctx context.Context, id domain.PileID, req domain.CagesRequest) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := tx.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if err := requireStage(task, domain.StageCleaningAccepted, domain.CodeRebarLayoutInvalid); err != nil {
		return err
	}
	design, err := tx.GetDesign(ctx, id)
	if err != nil {
		return err
	}
	if err := domain.ValidateRebar(req.Rebar, design.DesignDepth); err != nil {
		return err
	}
	if err := domain.ValidateSonic(req.Sonic); err != nil {
		return err
	}
	for _, r := range req.Rebar {
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidenceRebar, Range: domain.DepthRange{Start: r.Start, End: r.End},
			Value: r.End - r.Start, Time: task.LastTime, Generation: 1, Valid: true,
		}); err != nil {
			return err
		}
	}
	for _, st := range req.Sonic {
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidenceSonic, Range: domain.DepthRange{Start: st.Start, End: st.End},
			Value: st.End - st.Start, Time: task.LastTime, Generation: 1, Valid: true,
		}); err != nil {
			return err
		}
	}
	task.Stage = domain.StageCagesAccepted
	if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
		return err
	}
	return tx.Commit()
}

// Conduits validates and records the conduit assembly and water tightness.
func (s *Service) Conduits(ctx context.Context, id domain.PileID, req domain.ConduitsRequest) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := tx.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if err := requireStage(task, domain.StageCagesAccepted, domain.CodeConduitDuplicate); err != nil {
		return err
	}
	if err := domain.ValidateConduit(req.Segments, req.HoleDepth); err != nil {
		return err
	}
	var total int64
	for _, seg := range req.Segments {
		total += seg.Length
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidenceConduit, Range: domain.DepthRange{Start: 0, End: seg.Length},
			Value: seg.Length, Time: task.LastTime, Generation: 1, Valid: seg.Watertight,
		}); err != nil {
			return err
		}
	}
	if err := tx.InsertConduit(ctx, id, domain.ConduitAssembly{
		Segments: req.Segments, ActivePrefix: len(req.Segments), BottomDepth: total,
	}); err != nil {
		return err
	}
	task.Stage = domain.StageConduitsAccepted
	if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
		return err
	}
	return tx.Commit()
}

// acquireLease grants a device lease or returns a conflict, expiring stale
// leases first (the deterministic recovery path).
func acquireLease(tx *store.Tx, ctx context.Context, holder domain.PileID, dev domain.DeviceRequest, start domain.LogicalTime) (domain.DeviceLease, error) {
	if dev.LeaseEnd <= start {
		return domain.DeviceLease{}, domain.NewError(domain.CodeLeaseConflict, "lease end must be after start")
	}
	if _, err := tx.ExpireLeases(ctx, start); err != nil {
		return domain.DeviceLease{}, err
	}
	if _, found, err := tx.FindActiveLease(ctx, dev.DeviceType, dev.ResourceID, start); err != nil {
		return domain.DeviceLease{}, err
	} else if found {
		return domain.DeviceLease{}, domain.NewError(domain.CodeLeaseConflict, "device is already leased")
	}
	l := domain.DeviceLease{
		DeviceType: dev.DeviceType, ResourceID: dev.ResourceID, Holder: holder,
		Token: newToken(), Start: start, End: dev.LeaseEnd, Status: domain.LeaseActive,
	}
	if err := tx.AcquireLease(ctx, l); err != nil {
		return domain.DeviceLease{}, err
	}
	return l, nil
}

// deductBatch reserves litres from a batch, mapping optimistic conflicts to a
// stable insufficient-balance code.
func deductBatch(tx *store.Tx, ctx context.Context, batchID string, litres int64) error {
	b, err := tx.GetBatch(ctx, batchID)
	if err != nil {
		return err
	}
	if litres < 0 || litres > b.Available() {
		return domain.NewError(domain.CodeConcreteInsufficient, "insufficient concrete balance")
	}
	if err := tx.ReserveBatch(ctx, batchID, litres, b.Deducted); err != nil {
		if err == store.ErrVersionConflict {
			return domain.NewError(domain.CodeConcreteInsufficient, "concurrent batch deduction")
		}
		return err
	}
	return nil
}

// StartPour performs the first-pour base sealing atomically.
func (s *Service) StartPour(ctx context.Context, id domain.PileID, req domain.StartPourRequest) error {
	_, err := s.idemResult(ctx, req.OperationID, req, func(tx *store.Tx) (string, error) {
		task, err := tx.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		if err := requireStage(task, domain.StageConduitsAccepted, domain.CodeFirstPourInsufficient); err != nil {
			return "", err
		}
		design, err := tx.GetDesign(ctx, id)
		if err != nil {
			return "", err
		}
		conduit, err := tx.GetConduit(ctx, id)
		if err != nil {
			return "", err
		}
		if req.Litres < design.Pour.FirstPourVolume {
			return "", domain.NewError(domain.CodeFirstPourInsufficient, "first pour below the sealing volume")
		}
		area, err := domain.CrossArea(design.Diameter)
		if err != nil {
			return "", err
		}
		if _, err := acquireLease(tx, ctx, id, req.Device, req.Time); err != nil {
			return "", err
		}
		if err := deductBatch(tx, ctx, req.BatchID, req.Litres); err != nil {
			return "", err
		}
		theory, overpour, err := domain.TheoreticalLevel(design.DesignDepth, req.Litres, area)
		if err != nil {
			return "", err
		}
		embedment := conduit.BottomDepth - theory
		if embedment < design.Pour.MinEmbedment {
			return "", domain.NewError(domain.CodeFirstPourInsufficient, "first pour does not embed the conduit base")
		}
		if embedment > design.Pour.MaxEmbedment {
			return "", domain.NewError(domain.CodeEmbedmentOutOfRange, "conduit embedment exceeds the maximum")
		}
		seq, err := tx.NextTraceSeq(ctx, id)
		if err != nil {
			return "", err
		}
		entry := domain.PourTraceEntry{
			Seq: seq, OperationID: req.OperationID, Time: req.Time, EventType: domain.PourFirst,
			BatchLitres: req.Litres, TotalLitres: req.Litres, TheoryLevel: theory,
			ConduitPrefix: conduit.ActivePrefix, Embedment: embedment, Overpour: overpour,
		}
		if err := tx.InsertTrace(ctx, id, entry); err != nil {
			return "", err
		}
		if err := tx.InsertEvidence(ctx, id, domain.InspectionEvidence{
			Type: domain.EvidencePour, Range: domain.DepthRange{Start: 0, End: design.DesignDepth},
			Value: req.Litres, Time: req.Time, Generation: 1, Valid: true,
		}); err != nil {
			return "", err
		}
		task.Stage = domain.StagePoured
		task.LastTime = req.Time
		if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
			return "", err
		}
		return "", nil
	})
	return err
}

// PourEntry performs a continuous-pour increment.
func (s *Service) PourEntry(ctx context.Context, id domain.PileID, req domain.PourRequest) error {
	_, err := s.idemResult(ctx, req.OperationID, req, func(tx *store.Tx) (string, error) {
		task, err := tx.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		if err := requireStage(task, domain.StagePoured, domain.CodePourInterrupted); err != nil {
			return "", err
		}
		design, err := tx.GetDesign(ctx, id)
		if err != nil {
			return "", err
		}
		conduit, err := tx.GetConduit(ctx, id)
		if err != nil {
			return "", err
		}
		prev, ok, err := tx.LastTrace(ctx, id)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", domain.NewError(domain.CodePourInterrupted, "pour has not started")
		}
		if req.Time <= prev.Time {
			return "", domain.NewError(domain.CodePourInterrupted, "logical time must strictly increase")
		}
		if req.Time-prev.Time > design.Pour.ContinuousMaxGap {
			return "", domain.NewError(domain.CodePourInterrupted, "continuous pour gap exceeds the limit")
		}
		if req.Litres <= 0 {
			return "", domain.NewError(domain.CodeConcreteInsufficient, "pour volume must be positive")
		}
		area, err := domain.CrossArea(design.Diameter)
		if err != nil {
			return "", err
		}
		if _, err := acquireLease(tx, ctx, id, req.Device, req.Time); err != nil {
			return "", err
		}
		if err := deductBatch(tx, ctx, req.BatchID, req.Litres); err != nil {
			return "", err
		}
		total, err := fixed.Add(prev.TotalLitres, req.Litres)
		if err != nil {
			return "", domain.NewError(domain.CodeFixedPointOverflow, "cumulative volume overflows")
		}
		theory, overpour, err := domain.TheoreticalLevel(design.DesignDepth, total, area)
		if err != nil {
			return "", err
		}
		embedment := conduit.BottomDepth - theory
		if embedment < design.Pour.MinEmbedment || embedment > design.Pour.MaxEmbedment {
			return "", domain.NewError(domain.CodeEmbedmentOutOfRange, "conduit embedment out of range")
		}
		seq, err := tx.NextTraceSeq(ctx, id)
		if err != nil {
			return "", err
		}
		entry := domain.PourTraceEntry{
			Seq: seq, OperationID: req.OperationID, Time: req.Time, EventType: domain.PourContinue,
			BatchLitres: req.Litres, TotalLitres: total, TheoryLevel: theory,
			MeasuredLevel: req.MeasuredLevel, ConduitPrefix: conduit.ActivePrefix,
			Embedment: embedment, Overpour: overpour,
		}
		if err := tx.InsertTrace(ctx, id, entry); err != nil {
			return "", err
		}
		task.LastTime = req.Time
		if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
			return "", err
		}
		return "", nil
	})
	return err
}

// LevelReading records a concrete level re-measurement.
func (s *Service) LevelReading(ctx context.Context, id domain.PileID, req domain.LevelRequest) error {
	if _, failed := mapDeviceOutcome(req.DeviceOutcome); failed {
		// Scripted sounding-line failure: record a retry, do not advance state.
		return s.failDevice(ctx, id, req.OperationID, "sounding-line", req.DeviceOutcome, req.Time)
	}
	_, err := s.idemResult(ctx, req.OperationID, req, func(tx *store.Tx) (string, error) {
		task, err := tx.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		if err := requireStage(task, domain.StagePoured, domain.CodePourInterrupted); err != nil {
			return "", err
		}
		conduit, err := tx.GetConduit(ctx, id)
		if err != nil {
			return "", err
		}
		prev, ok, err := tx.LastTrace(ctx, id)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", domain.NewError(domain.CodePourInterrupted, "pour has not started")
		}
		if req.Time <= prev.Time {
			return "", domain.NewError(domain.CodePourInterrupted, "logical time must strictly increase")
		}
		if _, err := acquireLease(tx, ctx, id, req.Device, req.Time); err != nil {
			return "", err
		}
		seq, err := tx.NextTraceSeq(ctx, id)
		if err != nil {
			return "", err
		}
		entry := domain.PourTraceEntry{
			Seq: seq, OperationID: req.OperationID, Time: req.Time, EventType: domain.PourLevelReading,
			TotalLitres: prev.TotalLitres, TheoryLevel: prev.TheoryLevel,
			MeasuredLevel: req.MeasuredLevel, ConduitPrefix: conduit.ActivePrefix,
			Embedment: conduit.BottomDepth - prev.TheoryLevel, Overpour: prev.Overpour,
		}
		if err := tx.InsertTrace(ctx, id, entry); err != nil {
			return "", err
		}
		task.LastTime = req.Time
		if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
			return "", err
		}
		return "", nil
	})
	return err
}

// RemoveSegments removes trailing active conduit segments, preserving the
// original assembly's contiguous prefix and the embedment range.
func (s *Service) RemoveSegments(ctx context.Context, id domain.PileID, req domain.RemoveRequest) error {
	_, err := s.idemResult(ctx, req.OperationID, req, func(tx *store.Tx) (string, error) {
		task, err := tx.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		if err := requireStage(task, domain.StagePoured, domain.CodePourInterrupted); err != nil {
			return "", err
		}
		design, err := tx.GetDesign(ctx, id)
		if err != nil {
			return "", err
		}
		conduit, err := tx.GetConduit(ctx, id)
		if err != nil {
			return "", err
		}
		prev, ok, err := tx.LastTrace(ctx, id)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", domain.NewError(domain.CodePourInterrupted, "pour has not started")
		}
		if req.Time <= prev.Time {
			return "", domain.NewError(domain.CodePourInterrupted, "logical time must strictly increase")
		}
		if req.Count <= 0 || req.Count > conduit.ActivePrefix {
			return "", domain.NewError(domain.CodeConduitDuplicate, "cannot remove that many segments")
		}
		newPrefix := conduit.ActivePrefix - req.Count
		var newBottom int64
		for i := 0; i < newPrefix; i++ {
			newBottom += conduit.Segments[i].Length
		}
		embedment := newBottom - prev.TheoryLevel
		if embedment < design.Pour.MinEmbedment || embedment > design.Pour.MaxEmbedment {
			return "", domain.NewError(domain.CodeEmbedmentOutOfRange, "removal pushes embedment out of range")
		}
		if err := tx.UpdateConduit(ctx, id, domain.ConduitAssembly{
			Segments: conduit.Segments, ActivePrefix: newPrefix, BottomDepth: newBottom,
		}); err != nil {
			return "", err
		}
		seq, err := tx.NextTraceSeq(ctx, id)
		if err != nil {
			return "", err
		}
		entry := domain.PourTraceEntry{
			Seq: seq, OperationID: req.OperationID, Time: req.Time, EventType: domain.PourRemoveSegment,
			TotalLitres: prev.TotalLitres, TheoryLevel: prev.TheoryLevel,
			MeasuredLevel: prev.MeasuredLevel, ConduitPrefix: newPrefix,
			Embedment: embedment, Overpour: prev.Overpour,
		}
		if err := tx.InsertTrace(ctx, id, entry); err != nil {
			return "", err
		}
		task.LastTime = req.Time
		if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
			return "", err
		}
		return "", nil
	})
	return err
}

// FinishPour records pile-top finishing and closes the pour.
func (s *Service) FinishPour(ctx context.Context, id domain.PileID, req domain.FinishRequest) error {
	_, err := s.idemResult(ctx, req.OperationID, req, func(tx *store.Tx) (string, error) {
		task, err := tx.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		if err := requireStage(task, domain.StagePoured, domain.CodePourInterrupted); err != nil {
			return "", err
		}
		design, err := tx.GetDesign(ctx, id)
		if err != nil {
			return "", err
		}
		prev, ok, err := tx.LastTrace(ctx, id)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", domain.NewError(domain.CodePourInterrupted, "pour has not started")
		}
		if req.Time <= prev.Time {
			return "", domain.NewError(domain.CodePourInterrupted, "logical time must strictly increase")
		}
		if prev.Overpour < design.Overpour {
			return "", domain.NewError(domain.CodePourInterrupted, "overpour height has not reached the locked threshold")
		}
		seq, err := tx.NextTraceSeq(ctx, id)
		if err != nil {
			return "", err
		}
		entry := domain.PourTraceEntry{
			Seq: seq, OperationID: req.OperationID, Time: req.Time, EventType: domain.PourFinish,
			TotalLitres: prev.TotalLitres, TheoryLevel: prev.TheoryLevel,
			MeasuredLevel: prev.MeasuredLevel, ConduitPrefix: prev.ConduitPrefix,
			Embedment: prev.Embedment, Overpour: prev.Overpour,
		}
		if err := tx.InsertTrace(ctx, id, entry); err != nil {
			return "", err
		}
		task.Stage = domain.StageFinished
		task.LastTime = req.Time
		task.AgeDeadline = req.Time + design.AgePeriod
		if err := tx.UpdateTask(ctx, task, task.Version); err != nil {
			return "", err
		}
		return "", nil
	})
	return err
}

// Trace returns the append-only pour trace in sequence order.
func (s *Service) Trace(ctx context.Context, id domain.PileID) ([]domain.PourTraceEntry, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return tx.GetTrace(ctx, id)
}

var _ domain.TaskTrace = (*Service)(nil)
