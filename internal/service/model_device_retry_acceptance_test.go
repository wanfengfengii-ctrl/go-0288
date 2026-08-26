package service_test

import (
	"context"
	"testing"

	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/service"
)

func TestModel_DeviceRetryRecovery(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, *service.Service, domain.PileID)
	}{
		{
			name: "successful retry completes deferred level reading atomically",
			run: func(t *testing.T, svc *service.Service, id domain.PileID) {
				ctx := context.Background()
				beforeEvidence, err := svc.Evidence(ctx, id)
				if err != nil {
					t.Fatalf("evidence before retry: %v", err)
				}

				err = svc.LevelReading(ctx, id, domain.LevelRequest{
					OperationID: "lv1", Time: 200, MeasuredLevel: 5000, DeviceOutcome: "timeout",
					Device: domain.DeviceRequest{DeviceType: domain.DeviceSoundingLine, ResourceID: "SL1", LeaseEnd: 5000},
				})
				if !domain.IsCode(err, domain.CodeDeviceTimeout) {
					t.Fatalf("initial level reading err = %v, want DEVICE_TIMEOUT", err)
				}
				trace, err := svc.Trace(ctx, id)
				if err != nil {
					t.Fatalf("trace after timeout: %v", err)
				}
				task, err := svc.Task(ctx, id)
				if err != nil {
					t.Fatalf("task after timeout: %v", err)
				}
				if len(trace) != 1 || task.LastTime != 100 {
					t.Fatalf("timeout advanced state: trace len=%d last_time=%d, want 1 and 100", len(trace), task.LastTime)
				}

				if err := svc.Retry(ctx, id, "lv1-call", domain.RetryRequest{Time: 300, Outcome: "success", Reading: 5200}); err != nil {
					t.Fatalf("successful retry: %v", err)
				}
				trace, err = svc.Trace(ctx, id)
				if err != nil {
					t.Fatalf("trace after retry: %v", err)
				}
				if len(trace) != 2 {
					t.Fatalf("trace len = %d, want 2", len(trace))
				}
				got := trace[1]
				if got.EventType != domain.PourLevelReading || got.OperationID != "lv1" || got.Time != 300 || got.MeasuredLevel != 5200 {
					t.Fatalf("recovered trace = %+v, want level/lv1/time 300/reading 5200", got)
				}
				if got.TotalLitres != trace[0].TotalLitres || got.TheoryLevel != trace[0].TheoryLevel {
					t.Fatalf("recovered trace changed pour totals: first=%+v recovered=%+v", trace[0], got)
				}
				task, err = svc.Task(ctx, id)
				if err != nil {
					t.Fatalf("task after retry: %v", err)
				}
				if task.LastTime != 300 {
					t.Fatalf("last_time = %d, want 300", task.LastTime)
				}
				afterEvidence, err := svc.Evidence(ctx, id)
				if err != nil {
					t.Fatalf("evidence after retry: %v", err)
				}
				if len(afterEvidence) != len(beforeEvidence) {
					t.Fatalf("evidence len = %d, want unchanged %d (level retry must not become core evidence)", len(afterEvidence), len(beforeEvidence))
				}
				if err := svc.Retry(ctx, id, "lv1-call", domain.RetryRequest{Time: 301, Outcome: "success", Reading: 5201}); !domain.IsCode(err, domain.CodeDeviceRejected) {
					t.Fatalf("second retry err = %v, want DEVICE_REJECTED after retry record is cleared", err)
				}
			},
		},
		{
			name: "failed retry remains pending without advancing state",
			run: func(t *testing.T, svc *service.Service, id domain.PileID) {
				ctx := context.Background()
				err := svc.LevelReading(ctx, id, domain.LevelRequest{
					OperationID: "lv-fail", Time: 200, MeasuredLevel: 5100, DeviceOutcome: "timeout",
					Device: domain.DeviceRequest{DeviceType: domain.DeviceSoundingLine, ResourceID: "SL2", LeaseEnd: 5000},
				})
				if !domain.IsCode(err, domain.CodeDeviceTimeout) {
					t.Fatalf("initial level reading err = %v, want DEVICE_TIMEOUT", err)
				}
				if err := svc.Retry(ctx, id, "lv-fail-call", domain.RetryRequest{Time: 300, Outcome: "timeout"}); err != nil {
					t.Fatalf("record failed retry: %v", err)
				}
				trace, err := svc.Trace(ctx, id)
				if err != nil {
					t.Fatalf("trace after failed retry: %v", err)
				}
				task, err := svc.Task(ctx, id)
				if err != nil {
					t.Fatalf("task after failed retry: %v", err)
				}
				if len(trace) != 1 || task.LastTime != 100 {
					t.Fatalf("failed retry advanced state: trace len=%d last_time=%d, want 1 and 100", len(trace), task.LastTime)
				}
				if err := svc.Retry(ctx, id, "lv-fail-call", domain.RetryRequest{Time: 301, Outcome: "success", Reading: 5300}); err != nil {
					t.Fatalf("pending retry was not retained: %v", err)
				}
			},
		},
		{
			name: "retry is scoped to its pile",
			run: func(t *testing.T, svc *service.Service, id domain.PileID) {
				ctx := context.Background()
				err := svc.LevelReading(ctx, id, domain.LevelRequest{
					OperationID: "lv-owned", Time: 200, MeasuredLevel: 5000, DeviceOutcome: "timeout",
					Device: domain.DeviceRequest{DeviceType: domain.DeviceSoundingLine, ResourceID: "SL3", LeaseEnd: 5000},
				})
				if !domain.IsCode(err, domain.CodeDeviceTimeout) {
					t.Fatalf("initial level reading err = %v, want DEVICE_TIMEOUT", err)
				}
				other := fullSetup(t, svc)
				if err := svc.Retry(ctx, other, "lv-owned-call", domain.RetryRequest{Time: 300, Outcome: "success", Reading: 5400}); !domain.IsCode(err, domain.CodeDeviceRejected) {
					t.Fatalf("cross-pile retry err = %v, want DEVICE_REJECTED", err)
				}
				if err := svc.Retry(ctx, id, "lv-owned-call", domain.RetryRequest{Time: 300, Outcome: "success", Reading: 5400}); err != nil {
					t.Fatalf("owner could not recover retained retry: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newService(t)
			ctx := context.Background()
			id := fullSetup(t, svc)
			if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "B1", Initial: 100000}); err != nil {
				t.Fatalf("create batch: %v", err)
			}
			if err := svc.StartPour(ctx, id, domain.StartPourRequest{
				OperationID: "op1", Time: 100, BatchID: "B1", Litres: 2000,
				Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "P1", LeaseEnd: 1000},
			}); err != nil {
				t.Fatalf("start pour: %v", err)
			}
			tt.run(t, svc, id)
		})
	}
}
