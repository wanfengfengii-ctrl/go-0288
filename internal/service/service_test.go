package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/service"
	"deep-pile-pour-integrity-closure/internal/store"
)

func validDesign() domain.CreateTaskRequest {
	return domain.CreateTaskRequest{
		Pier: "P1", PileNo: "1", Summary: "P1-1 test pile",
		DesignDepth: 10000, Diameter: 1000,
		Layers: []domain.Layer{
			{Name: "soil", Start: 0, End: 5000},
			{Name: "rock", Start: 5000, End: 10000},
		},
		Rebar: []domain.RebarSegment{{Index: 0, Start: 0, End: 10000, Direction: domain.DirectionBottomUp}},
		Sonic: []domain.SonicTube{
			{ID: "S1", Start: 0, End: 10000, Neighbors: []string{"S2"}},
			{ID: "S2", Start: 0, End: 10000, Neighbors: []string{"S1"}},
		},
		Mud: domain.MudThresholds{
			SpecificGravityMin: 1_100_000, SpecificGravityMax: 1_300_000,
			ViscosityMin: 1_800_000, ViscosityMax: 2_500_000, SandContentMax: 40_000,
		},
		Cleaning: domain.CleaningThresholds{SedimentMax: 300, ApertureTolerance: 100},
		Pour: domain.PourWindow{
			FirstPourVolume: 2000, ContinuousMaxGap: 100,
			MinEmbedment: 400, MaxEmbedment: 6000,
		},
		Overpour:      500,
		LineAdjacency: [][]string{{"S1", "S2"}},
		Coring:        domain.CoringRules{MinCoresPerAnomaly: 1, CoreDepthStep: 1000},
		AgePeriod:     10,
		MaxRetries:    3,
	}
}

func newService(t *testing.T) (*service.Service, *store.SQLStore) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "piles.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return service.New(st), st
}

func fullSetup(t *testing.T, svc *service.Service) domain.PileID {
	return fullSetupDesign(t, svc, validDesign())
}

func fullSetupDesign(t *testing.T, svc *service.Service, d domain.CreateTaskRequest) domain.PileID {
	t.Helper()
	ctx := context.Background()
	id, err := svc.CreateTask(ctx, d)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Lock(ctx, id); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := svc.Borehole(ctx, id, domain.BoreholeRequest{
		FinalDepth: 10000,
		Aperture:   []domain.PointSample{{Depth: 0, Value: 1000}, {Depth: 5000, Value: 1005}, {Depth: 10000, Value: 995}},
		Sediment:   []domain.PointSample{{Depth: 0, Value: 10}, {Depth: 5000, Value: 20}, {Depth: 10000, Value: 250}},
	}); err != nil {
		t.Fatalf("borehole: %v", err)
	}
	if err := svc.AcceptCleaning(ctx, id, domain.CleaningRequest{Mud: []domain.MudSample{
		{Depth: 0, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
		{Depth: 5000, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
		{Depth: 10000, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
	}}); err != nil {
		t.Fatalf("cleaning: %v", err)
	}
	if err := svc.Cages(ctx, id, domain.CagesRequest{
		Rebar: []domain.RebarSegment{{Index: 0, Start: 0, End: 10000, Direction: domain.DirectionBottomUp}},
		Sonic: []domain.SonicTube{
			{ID: "S1", Start: 0, End: 10000, Neighbors: []string{"S2"}},
			{ID: "S2", Start: 0, End: 10000, Neighbors: []string{"S1"}},
		},
	}); err != nil {
		t.Fatalf("cages: %v", err)
	}
	if err := svc.Conduits(ctx, id, domain.ConduitsRequest{
		HoleDepth: 10000,
		Segments: []domain.ConduitSegment{
			{Index: 0, Length: 5000, Direction: domain.DirectionBottomUp, Watertight: true},
			{Index: 1, Length: 5000, Direction: domain.DirectionBottomUp, Watertight: true},
		},
	}); err != nil {
		t.Fatalf("conduits: %v", err)
	}
	return id
}

func TestCreateLockAndRelockRejected(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id, err := svc.CreateTask(ctx, validDesign())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Lock(ctx, id); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if _, err := svc.Lock(ctx, id); !domain.IsCode(err, domain.CodeDesignMismatch) {
		t.Fatalf("relock err = %v, want DESIGN_MISMATCH", err)
	}
}

func TestCreateRejectsInvalidLayers(t *testing.T) {
	svc, _ := newService(t)
	d := validDesign()
	d.Layers = []domain.Layer{{Name: "soil", Start: 0, End: 4000}} // gap to 10000
	if _, err := svc.CreateTask(context.Background(), d); !domain.IsCode(err, domain.CodeDesignMismatch) {
		t.Fatalf("err = %v, want DESIGN_MISMATCH", err)
	}
}

func TestBoreholeMissingSampleRejected(t *testing.T) {
	svc, _ := newService(t)
	id := fullSetup(t, svc)
	// Missing the 10000mm sample.
	if err := svc.Borehole(context.Background(), id, domain.BoreholeRequest{
		FinalDepth: 10000,
		Aperture:   []domain.PointSample{{Depth: 0, Value: 1000}, {Depth: 5000, Value: 1000}},
		Sediment:   []domain.PointSample{{Depth: 0, Value: 10}, {Depth: 5000, Value: 10}, {Depth: 10000, Value: 10}},
	}); !domain.IsCode(err, domain.CodeBoreholeConflict) {
		t.Fatalf("err = %v, want BOREHOLE_CONFLICT", err)
	}
}

func TestCagesRejectGap(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id, _ := svc.CreateTask(ctx, validDesign())
	_, _ = svc.Lock(ctx, id)
	_ = svc.Borehole(ctx, id, domain.BoreholeRequest{
		FinalDepth: 10000,
		Aperture:   []domain.PointSample{{Depth: 0, Value: 1000}, {Depth: 5000, Value: 1000}, {Depth: 10000, Value: 1000}},
		Sediment:   []domain.PointSample{{Depth: 0, Value: 10}, {Depth: 5000, Value: 10}, {Depth: 10000, Value: 10}},
	})
	_ = svc.AcceptCleaning(ctx, id, domain.CleaningRequest{Mud: []domain.MudSample{
		{Depth: 0, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
		{Depth: 5000, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
		{Depth: 10000, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
	}})
	if err := svc.Cages(ctx, id, domain.CagesRequest{
		Rebar: []domain.RebarSegment{{Index: 0, Start: 0, End: 8000, Direction: domain.DirectionBottomUp}},
		Sonic: []domain.SonicTube{
			{ID: "S1", Start: 0, End: 10000, Neighbors: []string{"S2"}},
			{ID: "S2", Start: 0, End: 10000, Neighbors: []string{"S1"}},
		},
	}); !domain.IsCode(err, domain.CodeRebarLayoutInvalid) {
		t.Fatalf("err = %v, want REBAR_LAYOUT_INVALID", err)
	}
}

func TestConduitRejectNotWatertight(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := fullSetup(t, svc)
	_ = id
	_ = ctx
	// Re-run conduits on a fresh pile is complex; instead validate a non-watertight assembly directly.
	if err := domain.ValidateConduit([]domain.ConduitSegment{
		{Index: 0, Length: 5000, Watertight: true},
		{Index: 1, Length: 5000, Watertight: false},
	}, 10000); !domain.IsCode(err, domain.CodeConduitDuplicate) {
		t.Fatalf("err = %v, want CONDUIT_DUPLICATE", err)
	}
}

func TestStartPourInsufficient(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := fullSetup(t, svc)
	if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 100000}); err != nil {
		t.Fatalf("batch: %v", err)
	}
	err := svc.StartPour(ctx, id, domain.StartPourRequest{
		OperationID: "op1", Time: 100, BatchID: "B1", Litres: 500, // below FirstPourVolume 2000
		Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
	})
	if !domain.IsCode(err, domain.CodeFirstPourInsufficient) {
		t.Fatalf("err = %v, want FIRST_POUR_INSUFFICIENT", err)
	}
	// No batch deduction and no lease retained.
	b, _ := svc.Batch(ctx, "B1")
	if b.Deducted != 0 {
		t.Fatalf("deducted = %d, want 0", b.Deducted)
	}
	leases, _ := svc.Leases(ctx, id)
	if len(leases) != 0 {
		t.Fatalf("leases = %d, want 0", len(leases))
	}
}

func TestStartPourAndTrace(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := fullSetup(t, svc)
	if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 100000}); err != nil {
		t.Fatalf("batch: %v", err)
	}
	if err := svc.StartPour(ctx, id, domain.StartPourRequest{
		OperationID: "op1", Time: 100, BatchID: "B1", Litres: 2000,
		Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
	}); err != nil {
		t.Fatalf("start pour: %v", err)
	}
	trace, err := svc.Trace(ctx, id)
	if err != nil {
		t.Fatalf("trace: %v", err)
	}
	if len(trace) != 1 {
		t.Fatalf("trace len = %d, want 1", len(trace))
	}
	if trace[0].TotalLitres != 2000 || trace[0].Embedment < 400 || trace[0].Embedment > 6000 {
		t.Fatalf("bad trace entry: %+v", trace[0])
	}
	b, _ := svc.Batch(ctx, "B1")
	if b.Deducted != 2000 {
		t.Fatalf("deducted = %d, want 2000", b.Deducted)
	}
}

func TestConcurrentBatchOverReserve(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	d2 := validDesign()
	d2.Pier, d2.PileNo = "P2", "2"
	id1 := fullSetup(t, svc)
	id2 := fullSetupDesign(t, svc, d2)
	if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 3000}); err != nil {
		t.Fatalf("batch: %v", err)
	}
	// Two distinct piles race to pour 2000 litres from a 3000-litre batch.
	var wg sync.WaitGroup
	results := make([]error, 2)
	mk := func(op string) domain.StartPourRequest {
		return domain.StartPourRequest{
			OperationID: op, Time: 100, BatchID: "B1", Litres: 2000,
			Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
		}
	}
	wg.Add(2)
	go func() { defer wg.Done(); results[0] = svc.StartPour(ctx, id1, mk("race1")) }()
	go func() { defer wg.Done(); results[1] = svc.StartPour(ctx, id2, mk("race2")) }()
	wg.Wait()
	ok := 0
	for _, e := range results {
		if e == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("successful pours = %d, want 1 (results %v %v)", ok, results[0], results[1])
	}
	b, _ := svc.Batch(ctx, "B1")
	if b.Deducted != 2000 || b.Available() != 1000 {
		t.Fatalf("batch state %+v, want deducted 2000 available 1000", b)
	}
}

func TestIdempotencyConflict(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := fullSetup(t, svc)
	_ = svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 100000})
	req := domain.StartPourRequest{
		OperationID: "idem", Time: 100, BatchID: "B1", Litres: 2000,
		Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
	}
	if err := svc.StartPour(ctx, id, req); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Same content: idempotent success, no second trace entry.
	if err := svc.StartPour(ctx, id, req); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	trace, _ := svc.Trace(ctx, id)
	if len(trace) != 1 {
		t.Fatalf("trace len = %d, want 1 after idempotent retry", len(trace))
	}
	// Different content: conflict.
	req.Litres = 2500
	if err := svc.StartPour(ctx, id, req); !domain.IsCode(err, domain.CodeIdempotencyConflict) {
		t.Fatalf("err = %v, want IDEMPOTENCY_CONFLICT", err)
	}
}

func TestDeviceRetryFlow(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := fullSetup(t, svc)
	_ = svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 100000})
	_ = svc.StartPour(ctx, id, domain.StartPourRequest{
		OperationID: "op1", Time: 100, BatchID: "B1", Litres: 2000,
		Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
	})
	// Scripted sounding-line timeout.
	err := svc.LevelReading(ctx, id, domain.LevelRequest{
		OperationID: "lv1", Time: 200, MeasuredLevel: 5000, DeviceOutcome: "timeout",
		Device: domain.DeviceRequest{DeviceType: domain.DeviceSoundingLine, ResourceID: "SL1", LeaseEnd: 5000},
	})
	if !domain.IsCode(err, domain.CodeDeviceTimeout) {
		t.Fatalf("err = %v, want DEVICE_TIMEOUT", err)
	}
	// State not advanced: still one trace entry.
	trace, _ := svc.Trace(ctx, id)
	if len(trace) != 1 {
		t.Fatalf("trace len = %d, want 1 (state must not advance)", len(trace))
	}
	// Retry with a success outcome resolves the call.
	if err := svc.Retry(ctx, id, "lv1-call", domain.RetryRequest{Time: 300, Outcome: "success", Reading: 5200}); err != nil {
		t.Fatalf("retry success: %v", err)
	}
}

func TestRestartRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "piles.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	svc := service.New(st)
	id := fullSetup(t, svc)
	_ = svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 100000})
	_ = svc.StartPour(ctx, id, domain.StartPourRequest{
		OperationID: "op1", Time: 100, BatchID: "B1", Litres: 2000,
		Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
	})
	st.Close()

	// Reopen and verify state survives.
	st2, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	svc2 := service.New(st2)
	trace, err := svc2.Trace(ctx, id)
	if err != nil {
		t.Fatalf("trace after restart: %v", err)
	}
	if len(trace) != 1 || trace[0].TotalLitres != 2000 {
		t.Fatalf("recovered trace %+v, want one 2000L entry", trace)
	}
	b, _ := svc2.Batch(ctx, "B1")
	if b.Deducted != 2000 {
		t.Fatalf("recovered batch deducted = %d, want 2000", b.Deducted)
	}
	task, _ := svc2.Task(ctx, id)
	if task.Stage != domain.StagePoured {
		t.Fatalf("recovered stage = %s, want poured", task.Stage)
	}
}

func TestAnomalyExpansion(t *testing.T) {
	d := validDesign().Snapshot(1)
	reinspect, lines := domain.ExpandAnomalies(d, []domain.LineResult{
		{Line: "S1", Anomalous: true, AnomalyRanges: []domain.DepthRange{{Start: 2000, End: 3000}}},
	})
	if len(reinspect) != 1 {
		t.Fatalf("reinspect = %+v, want one merged range", reinspect)
	}
	// 2000-3000 expands to layer boundaries 0-5000.
	if reinspect[0].Start != 0 || reinspect[0].End != 5000 {
		t.Fatalf("reinspect = %+v, want 0-5000", reinspect[0])
	}
	// Anomalous line S1 includes its neighbour S2.
	if len(lines) != 2 {
		t.Fatalf("lines = %v, want S1 and S2", lines)
	}
}

func TestTerminalSingleWriter(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := fullSetup(t, svc)
	// Finish the pour so generations are reachable.
	_ = svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 1000000})
	_ = svc.StartPour(ctx, id, domain.StartPourRequest{
		OperationID: "op1", Time: 100, BatchID: "B1", Litres: 2000,
		Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
	})
	// Overpour 500mm requires ~8247L; pour enough in one continue entry.
	_ = svc.PourEntry(ctx, id, domain.PourRequest{
		OperationID: "op2", Time: 200, BatchID: "B1", Litres: 7000,
		Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
	})
	_ = svc.FinishPour(ctx, id, domain.FinishRequest{OperationID: "op3", Time: 300})
	_ = svc.NewGeneration(ctx, id)
	_ = svc.Integrity(ctx, id, domain.IntegrityRequest{Generation: 1, Lines: []domain.LineResult{
		{Line: "S1", Anomalous: false}, {Line: "S2", Anomalous: false},
	}})
	// Two qualified reviewers approve.
	_ = svc.Review(ctx, id, domain.ReviewRequest{ReviewerID: "r1", Qualified: true, Approve: true})
	_ = svc.Review(ctx, id, domain.ReviewRequest{ReviewerID: "r2", Qualified: true, Approve: true})

	// Three-way race for the single terminal.
	var wg sync.WaitGroup
	types := []domain.TerminalType{domain.TerminalAccept, domain.TerminalQuarantine, domain.TerminalCancel}
	results := make([]error, 3)
	for i, tt := range types {
		wg.Add(1)
		go func(i int, tt domain.TerminalType) {
			defer wg.Done()
			_, results[i] = svc.Terminate(ctx, id, domain.DecisionRequest{ReviewerID: "r1", Type: tt, Basis: "basis"})
		}(i, tt)
	}
	wg.Wait()
	wins := 0
	for _, e := range results {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("terminal winners = %d, want 1 (results %v)", wins, results)
	}
	rec, ok, _ := svc.Terminal(ctx, id)
	if !ok {
		t.Fatalf("no terminal recorded")
	}
	if rec.CredentialNo == "" {
		t.Fatalf("terminal has empty credential")
	}
}

func TestSameReviewerRejected(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := fullSetup(t, svc)
	if err := svc.Review(ctx, id, domain.ReviewRequest{ReviewerID: "r1", Qualified: true, Approve: true}); err != nil {
		t.Fatalf("first review: %v", err)
	}
	if err := svc.Review(ctx, id, domain.ReviewRequest{ReviewerID: "r1", Qualified: true, Approve: true}); !domain.IsCode(err, domain.CodeDesignMismatch) {
		t.Fatalf("err = %v, want DESIGN_MISMATCH for duplicate reviewer", err)
	}
}

func TestAcceptRequiresTwoApprovals(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	id := fullSetup(t, svc)
	_ = svc.Review(ctx, id, domain.ReviewRequest{ReviewerID: "r1", Qualified: true, Approve: true})
	if _, err := svc.Terminate(ctx, id, domain.DecisionRequest{ReviewerID: "r1", Type: domain.TerminalAccept, Basis: "b"}); !domain.IsCode(err, domain.CodeDesignMismatch) {
		t.Fatalf("err = %v, want DESIGN_MISMATCH (needs two approvals)", err)
	}
}

func TestNotFoundReturnsErr(t *testing.T) {
	svc, _ := newService(t)
	if _, err := svc.Task(context.Background(), "nonexistent"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}
