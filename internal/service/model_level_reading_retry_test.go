package service_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"deep-pile-pour-integrity-closure/internal/api"
	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/service"
	"deep-pile-pour-integrity-closure/internal/store"
)

func TestModel_LevelReadingTimeoutReplayIsStable(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "piles.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := service.New(st)
	id := fullSetup(t, svc)
	if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 100000}); err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := svc.StartPour(ctx, id, domain.StartPourRequest{
		OperationID: "start", Time: 100, BatchID: "B1", Litres: 2000,
		Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
	}); err != nil {
		t.Fatalf("start pour: %v", err)
	}

	services := domain.Services{Design: svc, Trace: svc, Material: svc, Evidence: svc, Arbiter: svc, Store: st}
	handler := api.NewServer(st, services).Handler()
	type errorResponse struct {
		Code string `json:"code"`
	}
	type testCase struct {
		name         string
		operationID  string
		request      domain.LevelRequest
		wantStatus   int
		wantCode     string
		wantTraceLen int
		wantLastTime domain.LogicalTime
		checkRetry   bool
	}
	cases := []testCase{
		{
			name: "first timeout creates deterministic pending call", operationID: "lv1",
			request:    domain.LevelRequest{Time: 200, MeasuredLevel: 5000, DeviceOutcome: "timeout", Device: domain.DeviceRequest{DeviceType: domain.DeviceSoundingLine, ResourceID: "SL1", LeaseEnd: 5000}},
			wantStatus: http.StatusUnprocessableEntity, wantCode: string(domain.CodeDeviceTimeout), wantTraceLen: 1, wantLastTime: 100, checkRetry: true,
		},
		{
			name: "identical timeout replay returns the same device error", operationID: "lv1",
			request:    domain.LevelRequest{Time: 200, MeasuredLevel: 5000, DeviceOutcome: "timeout", Device: domain.DeviceRequest{DeviceType: domain.DeviceSoundingLine, ResourceID: "SL1", LeaseEnd: 5000}},
			wantStatus: http.StatusUnprocessableEntity, wantCode: string(domain.CodeDeviceTimeout), wantTraceLen: 1, wantLastTime: 100, checkRetry: true,
		},
		{
			name: "successful reading appends one level trace", operationID: "lv2",
			request:    domain.LevelRequest{Time: 200, MeasuredLevel: 5100, Device: domain.DeviceRequest{DeviceType: domain.DeviceSoundingLine, ResourceID: "SL1", LeaseEnd: 5000}},
			wantStatus: http.StatusOK, wantTraceLen: 2, wantLastTime: 200, checkRetry: true,
		},
		{
			name: "identical successful replay does not append another trace", operationID: "lv2",
			request:    domain.LevelRequest{Time: 200, MeasuredLevel: 5100, Device: domain.DeviceRequest{DeviceType: domain.DeviceSoundingLine, ResourceID: "SL1", LeaseEnd: 5000}},
			wantStatus: http.StatusOK, wantTraceLen: 2, wantLastTime: 200, checkRetry: true,
		},
		{
			name: "changed content under a completed operation conflicts", operationID: "lv2",
			request:    domain.LevelRequest{Time: 201, MeasuredLevel: 5200, Device: domain.DeviceRequest{DeviceType: domain.DeviceSoundingLine, ResourceID: "SL1", LeaseEnd: 5000}},
			wantStatus: http.StatusConflict, wantCode: string(domain.CodeIdempotencyConflict), wantTraceLen: 2, wantLastTime: 200, checkRetry: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.request)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/piles/%s/pour/level-readings", id), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", tc.operationID)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantCode != "" {
				var got errorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if got.Code != tc.wantCode {
					t.Fatalf("code = %q, want %q; body=%s", got.Code, tc.wantCode, rec.Body.String())
				}
			}

			trace, err := svc.Trace(ctx, id)
			if err != nil {
				t.Fatalf("read trace: %v", err)
			}
			if len(trace) != tc.wantTraceLen {
				t.Fatalf("trace length = %d, want %d: %+v", len(trace), tc.wantTraceLen, trace)
			}
			if tc.wantTraceLen == 2 && (trace[1].EventType != domain.PourLevelReading || trace[1].OperationID != "lv2") {
				t.Fatalf("second trace = %+v, want the single lv2 level reading", trace[1])
			}
			task, err := svc.Task(ctx, id)
			if err != nil {
				t.Fatalf("read task: %v", err)
			}
			if task.LastTime != tc.wantLastTime {
				t.Fatalf("LastTime = %d, want %d", task.LastTime, tc.wantLastTime)
			}
			if tc.checkRetry {
				tx, err := st.Begin(ctx)
				if err != nil {
					t.Fatalf("begin retry lookup: %v", err)
				}
				rc, err := tx.GetRetry(ctx, id, "lv1-call")
				_ = tx.Rollback()
				if err != nil {
					t.Fatalf("get lv1-call: %v", err)
				}
				if rc.ID != "lv1-call" || rc.Request != "sounding-line" || rc.Attempts != 1 || rc.NextRetry != 201 || rc.FailureCode != domain.CodeDeviceTimeout {
					t.Fatalf("retry call changed: %+v", rc)
				}
			}
		})
	}

	inspectionDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open inspection database: %v", err)
	}
	defer inspectionDB.Close()
	var retryCount int
	if err := inspectionDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM retries WHERE pile_id = ?", id).Scan(&retryCount); err != nil {
		t.Fatalf("count retry calls: %v", err)
	}
	if retryCount != 1 {
		t.Fatalf("retry call count = %d, want exactly 1", retryCount)
	}
}
