package service_test

import (
	"context"
	"testing"

	"deep-pile-pour-integrity-closure/internal/domain"
)

func TestModel_CoreResultLeaseFollowsGenerationValidation(t *testing.T) {
	tests := []struct {
		name           string
		rejection      string
		wantRejectCode domain.ErrorCode
	}{
		{name: "invalid stage leaves the rig free", rejection: "stage", wantRejectCode: domain.CodeGenerationConflict},
		{name: "stale generation leaves the rig free", rejection: "generation", wantRejectCode: domain.CodeGenerationConflict},
		{name: "valid result commits lease evidence and closure together"},
		{name: "genuine active rig lease is rejected", rejection: "lease", wantRejectCode: domain.CodeLeaseConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc, st := newService(t)
			design := validDesign()
			design.AgePeriod = 0
			id := fullSetupDesign(t, svc, design)

			advanceToReview := func(pileID domain.PileID, suffix string) {
				t.Helper()
				if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "batch-" + suffix, Initial: 100000}); err != nil {
					t.Fatalf("create batch: %v", err)
				}
				if err := svc.StartPour(ctx, pileID, domain.StartPourRequest{
					OperationID: "start-" + suffix, Time: 100, BatchID: "batch-" + suffix, Litres: 2000,
					Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "pump-" + suffix, LeaseEnd: 150},
				}); err != nil {
					t.Fatalf("start pour: %v", err)
				}
				if err := svc.PourEntry(ctx, pileID, domain.PourRequest{
					OperationID: "pour-middle-" + suffix, Time: 200, BatchID: "batch-" + suffix, Litres: 2500,
					Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "pump-" + suffix, LeaseEnd: 250},
				}); err != nil {
					t.Fatalf("middle pour: %v", err)
				}
				if err := svc.RemoveSegments(ctx, pileID, domain.RemoveRequest{
					OperationID: "remove-" + suffix, Time: 300, Count: 1,
				}); err != nil {
					t.Fatalf("remove conduit segment: %v", err)
				}
				if err := svc.PourEntry(ctx, pileID, domain.PourRequest{
					OperationID: "pour-final-" + suffix, Time: 400, BatchID: "batch-" + suffix, Litres: 3750,
					Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "pump-" + suffix, LeaseEnd: 450},
				}); err != nil {
					t.Fatalf("final pour: %v", err)
				}
				if err := svc.FinishPour(ctx, pileID, domain.FinishRequest{OperationID: "finish-" + suffix, Time: 500}); err != nil {
					t.Fatalf("finish pour: %v", err)
				}
				if err := svc.NewGeneration(ctx, pileID); err != nil {
					t.Fatalf("new generation: %v", err)
				}
				if err := svc.Integrity(ctx, pileID, domain.IntegrityRequest{
					Generation: 2,
					Lines: []domain.LineResult{{
						Line: "S1", Anomalous: true,
						AnomalyRanges: []domain.DepthRange{{Start: 2000, End: 3000}},
					}},
				}); err != nil {
					t.Fatalf("integrity result: %v", err)
				}
			}

			request := func(generation domain.Generation) domain.CoreRequest {
				return domain.CoreRequest{
					Generation: generation,
					Findings: []domain.CoreFinding{{
						Range:  domain.DepthRange{Start: 2000, End: 3000},
						Defect: "none", Severity: "none",
					}},
					Device: domain.DeviceRequest{
						DeviceType: domain.DeviceCoringRig, ResourceID: "core-rig-1", LeaseEnd: 1000,
					},
				}
			}

			if tt.rejection != "stage" {
				advanceToReview(id, "primary")
			}

			switch tt.rejection {
			case "stage":
				if err := svc.CoreResult(ctx, id, request(1)); !domain.IsCode(err, tt.wantRejectCode) {
					t.Fatalf("invalid-stage error = %v, want %s", err, tt.wantRejectCode)
				}
			case "generation":
				if err := svc.CoreResult(ctx, id, request(1)); !domain.IsCode(err, tt.wantRejectCode) {
					t.Fatalf("stale-generation error = %v, want %s", err, tt.wantRejectCode)
				}
			case "lease":
				owner := fullSetupDesign(t, svc, design)
				advanceToReview(owner, "owner")
				if err := svc.CoreResult(ctx, owner, request(2)); err != nil {
					t.Fatalf("lease owner core result: %v", err)
				}
				if err := svc.CoreResult(ctx, id, request(2)); !domain.IsCode(err, tt.wantRejectCode) {
					t.Fatalf("contending core result error = %v, want %s", err, tt.wantRejectCode)
				}
				leases, err := svc.Leases(ctx, id)
				if err != nil {
					t.Fatalf("query contender leases: %v", err)
				}
				for _, lease := range leases {
					if lease.DeviceType == domain.DeviceCoringRig {
						t.Fatalf("contender retained coring-rig lease after conflict: %+v", lease)
					}
				}
				return
			}

			if tt.wantRejectCode != "" {
				leases, err := svc.Leases(ctx, id)
				if err != nil {
					t.Fatalf("query leases after rejection: %v", err)
				}
				for _, lease := range leases {
					if lease.DeviceType == domain.DeviceCoringRig {
						t.Fatalf("rejected request retained coring-rig lease: %+v", lease)
					}
				}
				if tt.rejection == "stage" {
					advanceToReview(id, "primary")
				}
			}

			before, err := svc.Evidence(ctx, id)
			if err != nil {
				t.Fatalf("evidence before valid core result: %v", err)
			}
			if err := svc.CoreResult(ctx, id, request(2)); err != nil {
				t.Fatalf("valid current-generation core result: %v", err)
			}

			leases, err := svc.Leases(ctx, id)
			if err != nil {
				t.Fatalf("query leases after valid result: %v", err)
			}
			activeCoreLeases := 0
			for _, lease := range leases {
				if lease.DeviceType == domain.DeviceCoringRig && lease.ResourceID == "core-rig-1" && lease.Status == domain.LeaseActive {
					activeCoreLeases++
				}
			}
			if activeCoreLeases != 1 {
				t.Fatalf("leases after valid result = %+v, want one active core-rig-1 lease", leases)
			}
			after, err := svc.Evidence(ctx, id)
			if err != nil {
				t.Fatalf("evidence after valid core result: %v", err)
			}
			if len(after) != len(before)+1 {
				t.Fatalf("evidence count = %d, want %d", len(after), len(before)+1)
			}
			matchingCoreEvidence := 0
			for _, evidence := range after {
				if evidence.Type == domain.EvidenceCore && evidence.Generation == 2 && evidence.Valid &&
					evidence.Range == (domain.DepthRange{Start: 2000, End: 3000}) {
					matchingCoreEvidence++
				}
			}
			if matchingCoreEvidence != 1 {
				t.Fatalf("generation 2 core evidence count = %d, want 1; evidence = %+v", matchingCoreEvidence, after)
			}

			tx, err := st.Begin(ctx)
			if err != nil {
				t.Fatalf("begin generation query: %v", err)
			}
			defer tx.Rollback()
			generation, err := tx.GetGeneration(ctx, id, 2)
			if err != nil {
				t.Fatalf("get generation: %v", err)
			}
			if generation.Conclusion != "closed" || len(generation.ReinspectSet) != 0 {
				t.Fatalf("generation after core result = %+v, want closed with empty reinspect set", generation)
			}
		})
	}
}
