package service_test

import (
	"context"
	"testing"

	"deep-pile-pour-integrity-closure/internal/domain"
)

func TestModel_IdempotencyScopeIncludesPileAndOperation(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	firstPile := fullSetup(t, svc)
	secondDesign := validDesign()
	secondDesign.Pier, secondDesign.PileNo, secondDesign.Summary = "P2", "2", "P2-2 test pile"
	secondPile := fullSetupDesign(t, svc, secondDesign)
	if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "shared-batch", Initial: 10000}); err != nil {
		t.Fatalf("create batch: %v", err)
	}

	original := domain.StartPourRequest{
		OperationID: "shared-operation",
		Time:        100,
		BatchID:     "shared-batch",
		Litres:      2000,
		Device: domain.DeviceRequest{
			DeviceType: domain.DeviceConcretePump,
			ResourceID: "shared-pump",
			LeaseEnd:   1000,
		},
	}
	changed := original
	changed.Litres = 2500

	cases := []struct {
		name          string
		pile          domain.PileID
		request       domain.StartPourRequest
		wantCode      domain.ErrorCode
		wantTraceLens [2]int
		wantDeducted  int64
	}{
		{
			name:          "initial operation executes",
			pile:          firstPile,
			request:       original,
			wantTraceLens: [2]int{1, 0},
			wantDeducted:  2000,
		},
		{
			name:          "same pile same operation and content replays",
			pile:          firstPile,
			request:       original,
			wantTraceLens: [2]int{1, 0},
			wantDeducted:  2000,
		},
		{
			name:          "same pile same operation with different content conflicts",
			pile:          firstPile,
			request:       changed,
			wantCode:      domain.CodeIdempotencyConflict,
			wantTraceLens: [2]int{1, 0},
			wantDeducted:  2000,
		},
		{
			name:          "same operation on another pile is executed rather than replayed",
			pile:          secondPile,
			request:       original,
			wantCode:      domain.CodeLeaseConflict,
			wantTraceLens: [2]int{1, 0},
			wantDeducted:  2000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.StartPour(ctx, tc.pile, tc.request)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("StartPour() error = %v, want nil", err)
				}
			} else if !domain.IsCode(err, tc.wantCode) {
				t.Fatalf("StartPour() error = %v, want code %s", err, tc.wantCode)
			}

			for i, pile := range []domain.PileID{firstPile, secondPile} {
				trace, traceErr := svc.Trace(ctx, pile)
				if traceErr != nil {
					t.Fatalf("Trace(%s) error = %v", pile, traceErr)
				}
				if len(trace) != tc.wantTraceLens[i] {
					t.Fatalf("Trace(%s) length = %d, want %d", pile, len(trace), tc.wantTraceLens[i])
				}
			}

			batch, batchErr := svc.Batch(ctx, "shared-batch")
			if batchErr != nil {
				t.Fatalf("Batch() error = %v", batchErr)
			}
			if batch.Deducted != tc.wantDeducted {
				t.Fatalf("batch deducted = %d, want %d", batch.Deducted, tc.wantDeducted)
			}
		})
	}
}
