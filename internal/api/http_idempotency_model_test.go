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

func TestModel_HTTPIdempotencyKeyBoundary(t *testing.T) {
	type errorResponse struct {
		Code string `json:"code"`
	}
	tests := []struct {
		name       string
		second     domain.PourRequest
		wantStatus int
		wantCode   string
	}{
		{
			name: "same key and content replays the successful response",
			second: domain.PourRequest{
				OperationID: "body-operation-1", Time: 110, BatchID: "batch-1", Litres: 1000,
				Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "pump-entry", LeaseEnd: 115},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "body operation number is not a second idempotency boundary",
			second: domain.PourRequest{
				OperationID: "body-operation-2", Time: 110, BatchID: "batch-1", Litres: 1000,
				Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "pump-entry", LeaseEnd: 115},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "same key with changed normalized payload conflicts atomically",
			second: domain.PourRequest{
				OperationID: "body-operation-2", Time: 120, BatchID: "batch-1", Litres: 1000,
				Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "pump-entry", LeaseEnd: 125},
			},
			wantStatus: http.StatusConflict,
			wantCode:   string(domain.CodeIdempotencyConflict),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st, err := store.Open(filepath.Join(t.TempDir(), "piles.db"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = st.Close() })
			svc := service.New(st)

			design := domain.CreateTaskRequest{
				Pier: "P1", PileNo: "1", Summary: "idempotency model pile",
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
			pileID, err := svc.CreateTask(ctx, design)
			if err != nil {
				t.Fatalf("create task: %v", err)
			}
			if _, err := svc.Lock(ctx, pileID); err != nil {
				t.Fatalf("lock task: %v", err)
			}
			points := []domain.PointSample{{Depth: 0, Value: 1000}, {Depth: 5000, Value: 1000}, {Depth: 10000, Value: 1000}}
			sediment := []domain.PointSample{{Depth: 0, Value: 10}, {Depth: 5000, Value: 10}, {Depth: 10000, Value: 10}}
			if err := svc.Borehole(ctx, pileID, domain.BoreholeRequest{FinalDepth: 10000, Aperture: points, Sediment: sediment}); err != nil {
				t.Fatalf("borehole checks: %v", err)
			}
			mud := []domain.MudSample{
				{Depth: 0, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
				{Depth: 5000, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
				{Depth: 10000, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
			}
			if err := svc.AcceptCleaning(ctx, pileID, domain.CleaningRequest{Mud: mud}); err != nil {
				t.Fatalf("cleaning acceptance: %v", err)
			}
			if err := svc.Cages(ctx, pileID, domain.CagesRequest{Rebar: design.Rebar, Sonic: design.Sonic}); err != nil {
				t.Fatalf("cages: %v", err)
			}
			segments := []domain.ConduitSegment{
				{Index: 0, Length: 5000, Direction: domain.DirectionBottomUp, Watertight: true},
				{Index: 1, Length: 5000, Direction: domain.DirectionBottomUp, Watertight: true},
			}
			if err := svc.Conduits(ctx, pileID, domain.ConduitsRequest{Segments: segments, HoleDepth: 10000}); err != nil {
				t.Fatalf("conduits: %v", err)
			}
			if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "batch-1", Initial: 10000}); err != nil {
				t.Fatalf("create batch: %v", err)
			}
			if err := svc.StartPour(ctx, pileID, domain.StartPourRequest{
				OperationID: "start-operation", Time: 100, BatchID: "batch-1", Litres: 2000,
				Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "pump-start", LeaseEnd: 105},
			}); err != nil {
				t.Fatalf("start pour: %v", err)
			}

			handler := api.NewServer(st, domain.Services{
				Design: svc, Trace: svc, Material: svc, Evidence: svc, Arbiter: svc, Store: st,
			}).Handler()
			first := domain.PourRequest{
				OperationID: "body-operation-1", Time: 110, BatchID: "batch-1", Litres: 1000,
				Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "pump-entry", LeaseEnd: 115},
			}
			post := func(body domain.PourRequest) *httptest.ResponseRecorder {
				t.Helper()
				encoded, err := json.Marshal(body)
				if err != nil {
					t.Fatalf("marshal request: %v", err)
				}
				req := httptest.NewRequest(http.MethodPost, "/v1/piles/"+string(pileID)+"/pour/entries", bytes.NewReader(encoded))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", "http-retry-key")
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				return rec
			}

			if rec := post(first); rec.Code != http.StatusOK {
				t.Fatalf("first entry status = %d, body = %s", rec.Code, rec.Body.String())
			}
			traceBefore, err := svc.Trace(ctx, pileID)
			if err != nil {
				t.Fatalf("trace before retry: %v", err)
			}
			batchBefore, err := svc.Batch(ctx, "batch-1")
			if err != nil {
				t.Fatalf("batch before retry: %v", err)
			}
			leasesBefore, err := svc.Leases(ctx, pileID)
			if err != nil {
				t.Fatalf("leases before retry: %v", err)
			}

			rec := post(tt.second)
			if rec.Code != tt.wantStatus {
				t.Fatalf("retry status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				var got errorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if got.Code != tt.wantCode {
					t.Fatalf("error code = %q, want %q", got.Code, tt.wantCode)
				}
			}

			traceAfter, err := svc.Trace(ctx, pileID)
			if err != nil {
				t.Fatalf("trace after retry: %v", err)
			}
			batchAfter, err := svc.Batch(ctx, "batch-1")
			if err != nil {
				t.Fatalf("batch after retry: %v", err)
			}
			leasesAfter, err := svc.Leases(ctx, pileID)
			if err != nil {
				t.Fatalf("leases after retry: %v", err)
			}
			if len(traceBefore) != 2 || batchBefore.Deducted != 3000 {
				t.Fatalf("bad precondition: trace=%d deducted=%d, want 2 and 3000", len(traceBefore), batchBefore.Deducted)
			}
			if len(traceAfter) != len(traceBefore) {
				t.Errorf("retry appended trace: before=%d after=%d", len(traceBefore), len(traceAfter))
			}
			if batchAfter.Deducted != batchBefore.Deducted {
				t.Errorf("retry deducted batch: before=%d after=%d", batchBefore.Deducted, batchAfter.Deducted)
			}
			if len(leasesAfter) != len(leasesBefore) {
				t.Errorf("retry acquired lease: before=%d after=%d", len(leasesBefore), len(leasesAfter))
			}
		})
	}
}
