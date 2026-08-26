package service_test

import (
	"context"
	"testing"

	"deep-pile-pour-integrity-closure/internal/domain"
)

func TestModel_StartPourTransactionBoundary(t *testing.T) {
	tests := []struct {
		name     string
		litres   int64
		wantCode domain.ErrorCode
	}{
		{name: "embedment above design maximum rolls back", litres: 5000, wantCode: domain.CodeEmbedmentOutOfRange},
		{name: "first pour below sealing volume rolls back", litres: 500, wantCode: domain.CodeFirstPourInsufficient},
		{name: "valid first pour commits once", litres: 2000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newService(t)
			ctx := context.Background()
			pileID := fullSetup(t, svc)
			if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "acceptance-batch", Initial: 10000}); err != nil {
				t.Fatalf("create batch: %v", err)
			}

			beforeTask, err := svc.Task(ctx, pileID)
			if err != nil {
				t.Fatalf("task before first pour: %v", err)
			}
			beforeEvidence, err := svc.Evidence(ctx, pileID)
			if err != nil {
				t.Fatalf("evidence before first pour: %v", err)
			}

			req := domain.StartPourRequest{
				OperationID: "start-pour-transaction",
				Time:        100,
				BatchID:     "acceptance-batch",
				Litres:      tt.litres,
				Device: domain.DeviceRequest{
					DeviceType: domain.DeviceConcretePump,
					ResourceID: "acceptance-pump",
					LeaseEnd:   1000,
				},
			}
			err = svc.StartPour(ctx, pileID, req)

			if tt.wantCode != "" {
				if !domain.IsCode(err, tt.wantCode) {
					t.Fatalf("StartPour error = %v, want %s", err, tt.wantCode)
				}
				batch, batchErr := svc.Batch(ctx, req.BatchID)
				if batchErr != nil {
					t.Fatalf("batch after rejection: %v", batchErr)
				}
				if batch.Deducted != 0 {
					t.Errorf("rejected first pour deducted %d litres, want 0", batch.Deducted)
				}
				leases, leaseErr := svc.Leases(ctx, pileID)
				if leaseErr != nil {
					t.Fatalf("leases after rejection: %v", leaseErr)
				}
				if len(leases) != 0 {
					t.Errorf("rejected first pour retained leases %+v, want none", leases)
				}
				trace, traceErr := svc.Trace(ctx, pileID)
				if traceErr != nil {
					t.Fatalf("trace after rejection: %v", traceErr)
				}
				if len(trace) != 0 {
					t.Errorf("rejected first pour appended trace %+v, want none", trace)
				}
				evidence, evidenceErr := svc.Evidence(ctx, pileID)
				if evidenceErr != nil {
					t.Fatalf("evidence after rejection: %v", evidenceErr)
				}
				if len(evidence) != len(beforeEvidence) {
					t.Errorf("rejected first pour changed evidence count from %d to %d", len(beforeEvidence), len(evidence))
				}
				task, taskErr := svc.Task(ctx, pileID)
				if taskErr != nil {
					t.Fatalf("task after rejection: %v", taskErr)
				}
				if task != beforeTask {
					t.Errorf("rejected first pour changed task from %+v to %+v", beforeTask, task)
				}

				// A rejected request must not leave an idempotency record either.
				req.Litres = 2000
				if retryErr := svc.StartPour(ctx, pileID, req); retryErr != nil {
					t.Fatalf("corrected request with the same operation ID: %v", retryErr)
				}
			} else if err != nil {
				t.Fatalf("valid StartPour: %v", err)
			}

			// Replaying the successful request must not duplicate any committed state.
			if err := svc.StartPour(ctx, pileID, req); err != nil {
				t.Fatalf("idempotent replay: %v", err)
			}
			batch, err := svc.Batch(ctx, req.BatchID)
			if err != nil {
				t.Fatalf("committed batch: %v", err)
			}
			if batch.Deducted != 2000 {
				t.Errorf("committed batch deduction = %d, want 2000", batch.Deducted)
			}
			leases, err := svc.Leases(ctx, pileID)
			if err != nil {
				t.Fatalf("committed leases: %v", err)
			}
			if len(leases) != 1 || leases[0].Status != domain.LeaseActive || leases[0].ResourceID != req.Device.ResourceID {
				t.Errorf("committed leases = %+v, want one active acceptance-pump lease", leases)
			}
			trace, err := svc.Trace(ctx, pileID)
			if err != nil {
				t.Fatalf("committed trace: %v", err)
			}
			if len(trace) != 1 || trace[0].EventType != domain.PourFirst || trace[0].BatchLitres != 2000 {
				t.Errorf("committed trace = %+v, want one 2000-litre first-pour entry", trace)
			}
			evidence, err := svc.Evidence(ctx, pileID)
			if err != nil {
				t.Fatalf("committed evidence: %v", err)
			}
			if len(evidence) != len(beforeEvidence)+1 {
				t.Errorf("committed evidence count = %d, want %d", len(evidence), len(beforeEvidence)+1)
			}
			task, err := svc.Task(ctx, pileID)
			if err != nil {
				t.Fatalf("committed task: %v", err)
			}
			if task.Stage != domain.StagePoured || task.LastTime != req.Time || task.Version != beforeTask.Version+1 {
				t.Errorf("committed task = %+v, want poured at time %d with version %d", task, req.Time, beforeTask.Version+1)
			}
		})
	}
}
