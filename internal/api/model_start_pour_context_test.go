package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"deep-pile-pour-integrity-closure/internal/api"
	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/service"
	"deep-pile-pour-integrity-closure/internal/store"
)

func TestModel_StartPourHonorsRequestCancellation(t *testing.T) {
	cases := []struct {
		name     string
		canceled bool
		persist  bool
	}{
		{name: "canceled request rolls back every first-pour write", canceled: true, persist: false},
		{name: "live request atomically saves the first pour", canceled: false, persist: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "piles.db"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = st.Close() })
			svc := service.New(st)
			ctx := context.Background()

			design := domain.CreateTaskRequest{
				Pier: "P1", PileNo: "1", Summary: "request cancellation",
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
				Overpour: 500, LineAdjacency: [][]string{{"S1", "S2"}},
				Coring:    domain.CoringRules{MinCoresPerAnomaly: 1, CoreDepthStep: 1000},
				AgePeriod: 10, MaxRetries: 3,
			}
			id, err := svc.CreateTask(ctx, design)
			if err != nil {
				t.Fatalf("create task: %v", err)
			}
			if _, err := svc.Lock(ctx, id); err != nil {
				t.Fatalf("lock task: %v", err)
			}
			points := []domain.PointSample{{Depth: 0, Value: 1000}, {Depth: 5000, Value: 1000}, {Depth: 10000, Value: 1000}}
			if err := svc.Borehole(ctx, id, domain.BoreholeRequest{
				FinalDepth: 10000, Aperture: points,
				Sediment: []domain.PointSample{{Depth: 0, Value: 10}, {Depth: 5000, Value: 10}, {Depth: 10000, Value: 10}},
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
			if err := svc.Cages(ctx, id, domain.CagesRequest{Rebar: design.Rebar, Sonic: design.Sonic}); err != nil {
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
			if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 10000}); err != nil {
				t.Fatalf("create batch: %v", err)
			}

			services := domain.Services{Design: svc, Trace: svc, Material: svc, Evidence: svc, Arbiter: svc, Store: st}
			handler := api.NewServer(st, services).Handler()
			pour := domain.StartPourRequest{
				OperationID: "first-pour-op", Time: 100, BatchID: "B1", Litres: 2000,
				Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "pump-1", LeaseEnd: 1000},
			}
			body, err := json.Marshal(pour)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/piles/"+string(id)+"/pour/start", bytes.NewReader(body))
			if tc.canceled {
				canceledCtx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(canceledCtx)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if tc.persist && rec.Code != http.StatusOK {
				t.Fatalf("live request status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if !tc.persist && rec.Code == http.StatusOK {
				t.Fatalf("canceled request unexpectedly succeeded: %s", rec.Body.String())
			}

			batch, err := svc.Batch(ctx, "B1")
			if err != nil {
				t.Fatalf("read batch: %v", err)
			}
			wantDeducted := int64(0)
			wantWrites := 0
			wantStage := domain.StageConduitsAccepted
			if tc.persist {
				wantDeducted, wantWrites, wantStage = 2000, 1, domain.StagePoured
			}
			if batch.Deducted != wantDeducted {
				t.Errorf("batch deducted = %d, want %d", batch.Deducted, wantDeducted)
			}
			leases, err := svc.Leases(ctx, id)
			if err != nil {
				t.Fatalf("read leases: %v", err)
			}
			if len(leases) != wantWrites {
				t.Errorf("lease count = %d, want %d", len(leases), wantWrites)
			}
			trace, err := svc.Trace(ctx, id)
			if err != nil {
				t.Fatalf("read trace: %v", err)
			}
			if len(trace) != wantWrites {
				t.Errorf("trace count = %d, want %d", len(trace), wantWrites)
			}
			evidence, err := svc.Evidence(ctx, id)
			if err != nil {
				t.Fatalf("read evidence: %v", err)
			}
			pourEvidence := 0
			for _, item := range evidence {
				if item.Type == domain.EvidencePour {
					pourEvidence++
				}
			}
			if pourEvidence != wantWrites {
				t.Errorf("pour evidence count = %d, want %d", pourEvidence, wantWrites)
			}
			task, err := svc.Task(ctx, id)
			if err != nil {
				t.Fatalf("read task: %v", err)
			}
			if task.Stage != wantStage {
				t.Errorf("task stage = %q, want %q", task.Stage, wantStage)
			}

			if tc.canceled {
				retry := httptest.NewRequest(http.MethodPost, "/v1/piles/"+string(id)+"/pour/start", bytes.NewReader(body))
				retryRec := httptest.NewRecorder()
				handler.ServeHTTP(retryRec, retry)
				if retryRec.Code != http.StatusOK {
					t.Fatalf("retry after cancellation status = %d, body = %s", retryRec.Code, retryRec.Body.String())
				}
				retriedBatch, err := svc.Batch(ctx, "B1")
				if err != nil {
					t.Fatalf("read batch after retry: %v", err)
				}
				if retriedBatch.Deducted != 2000 {
					t.Errorf("batch deducted after retry = %d, want 2000", retriedBatch.Deducted)
				}
			}
		})
	}
}
