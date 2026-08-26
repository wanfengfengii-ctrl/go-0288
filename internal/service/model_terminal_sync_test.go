package service_test

import (
	"context"
	"sync"
	"testing"

	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/service"
)

func TestModel_TerminalDecisionSynchronizesTask(t *testing.T) {
	ctx := context.Background()

	terminalReady := func(t *testing.T) (*service.Service, domain.PileID) {
		t.Helper()
		svc, _ := newService(t)
		id := fullSetup(t, svc)
		if err := svc.CreateBatch(ctx, domain.ConcreteBatch{ID: "terminal-sync-batch", Initial: 1000000}); err != nil {
			t.Fatalf("create batch: %v", err)
		}
		if err := svc.StartPour(ctx, id, domain.StartPourRequest{
			OperationID: "terminal-sync-start", Time: 100, BatchID: "terminal-sync-batch", Litres: 2000,
			Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "terminal-sync-pump-2", LeaseEnd: 1000},
		}); err != nil {
			t.Fatalf("start pour: %v", err)
		}
		if err := svc.PourEntry(ctx, id, domain.PourRequest{
			OperationID: "terminal-sync-continue-1", Time: 150, BatchID: "terminal-sync-batch", Litres: 2500,
			Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "terminal-sync-pump", LeaseEnd: 1000},
		}); err != nil {
			t.Fatalf("first continue pour: %v", err)
		}
		if err := svc.RemoveSegments(ctx, id, domain.RemoveRequest{OperationID: "terminal-sync-remove", Time: 200, Count: 1}); err != nil {
			t.Fatalf("remove conduit segment: %v", err)
		}
		if err := svc.PourEntry(ctx, id, domain.PourRequest{
			OperationID: "terminal-sync-continue-2", Time: 250, BatchID: "terminal-sync-batch", Litres: 3800,
			Device: domain.DeviceRequest{DeviceType: domain.DeviceConcretePump, ResourceID: "terminal-sync-pump-3", LeaseEnd: 1000},
		}); err != nil {
			t.Fatalf("second continue pour: %v", err)
		}
		if err := svc.FinishPour(ctx, id, domain.FinishRequest{OperationID: "terminal-sync-finish", Time: 300}); err != nil {
			t.Fatalf("finish pour: %v", err)
		}
		if err := svc.NewGeneration(ctx, id); err != nil {
			t.Fatalf("start inspection generation: %v", err)
		}
		return svc, id
	}

	cases := []struct {
		name string
		run  func(*testing.T, *service.Service, domain.PileID)
	}{
		{
			name: "accepted terminal atomically updates task detail",
			run: func(t *testing.T, svc *service.Service, id domain.PileID) {
				for _, reviewer := range []string{"accept-reviewer-1", "accept-reviewer-2"} {
					if err := svc.Review(ctx, id, domain.ReviewRequest{ReviewerID: reviewer, Qualified: true, Approve: true}); err != nil {
						t.Fatalf("review %s: %v", reviewer, err)
					}
				}
				rec, err := svc.Terminate(ctx, id, domain.DecisionRequest{ReviewerID: "accept-reviewer-1", Type: domain.TerminalAccept, Basis: "dual approval"})
				if err != nil {
					t.Fatalf("terminate: %v", err)
				}
				if rec.CredentialNo == "" {
					t.Fatal("terminal credential is empty")
				}
				task, err := svc.Task(ctx, id)
				if err != nil {
					t.Fatalf("task detail: %v", err)
				}
				if task.Stage != domain.StageTerminal || task.Terminal != rec.Type {
					t.Fatalf("task detail = {stage:%q terminal:%q}, terminal record type = %q", task.Stage, task.Terminal, rec.Type)
				}
			},
		},
		{
			name: "rejected acceptance leaves inspection stage unchanged",
			run: func(t *testing.T, svc *service.Service, id domain.PileID) {
				if err := svc.Review(ctx, id, domain.ReviewRequest{ReviewerID: "only-reviewer", Qualified: true, Approve: true}); err != nil {
					t.Fatalf("review: %v", err)
				}
				if _, err := svc.Terminate(ctx, id, domain.DecisionRequest{ReviewerID: "only-reviewer", Type: domain.TerminalAccept, Basis: "insufficient approval"}); !domain.IsCode(err, domain.CodeDesignMismatch) {
					t.Fatalf("terminate error = %v, want DESIGN_MISMATCH", err)
				}
				if _, found, err := svc.Terminal(ctx, id); err != nil || found {
					t.Fatalf("terminal after rejected decision = {found:%v err:%v}, want absent", found, err)
				}
				task, err := svc.Task(ctx, id)
				if err != nil {
					t.Fatalf("task detail: %v", err)
				}
				if task.Stage != domain.StageInspected || task.Terminal != "" {
					t.Fatalf("task after rejected decision = {stage:%q terminal:%q}, want inspected with empty terminal", task.Stage, task.Terminal)
				}
			},
		},
		{
			name: "concurrent contenders share one credential and one task outcome",
			run: func(t *testing.T, svc *service.Service, id domain.PileID) {
				for _, reviewer := range []string{"race-reviewer-1", "race-reviewer-2"} {
					if err := svc.Review(ctx, id, domain.ReviewRequest{ReviewerID: reviewer, Qualified: true, Approve: true}); err != nil {
						t.Fatalf("review %s: %v", reviewer, err)
					}
				}

				const contenders = 8
				records := make([]domain.TerminalRecord, contenders)
				errs := make([]error, contenders)
				var wg sync.WaitGroup
				for i := range records {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						records[i], errs[i] = svc.Terminate(ctx, id, domain.DecisionRequest{ReviewerID: "race-reviewer-1", Type: domain.TerminalAccept, Basis: "concurrent dual approval"})
					}(i)
				}
				wg.Wait()

				winners := 0
				credential := ""
				for i, err := range errs {
					if err == nil {
						winners++
						credential = records[i].CredentialNo
						continue
					}
					if !domain.IsCode(err, domain.CodeTerminalAlreadyDecided) {
						t.Fatalf("contender %d error = %v, want TERMINAL_ALREADY_DECIDED", i, err)
					}
				}
				if winners != 1 || credential == "" {
					t.Fatalf("winning decisions = %d, credential = %q; want exactly one non-empty credential", winners, credential)
				}
				for i, rec := range records {
					if rec.CredentialNo != credential || rec.Type != domain.TerminalAccept {
						t.Fatalf("contender %d record = %+v, want existing credential %q and accept type", i, rec, credential)
					}
				}
				stored, found, err := svc.Terminal(ctx, id)
				if err != nil || !found || stored.CredentialNo != credential {
					t.Fatalf("stored terminal = %+v, found = %v, err = %v; want credential %q", stored, found, err, credential)
				}
				task, err := svc.Task(ctx, id)
				if err != nil {
					t.Fatalf("task detail: %v", err)
				}
				if task.Stage != domain.StageTerminal || task.Terminal != stored.Type {
					t.Fatalf("task detail = {stage:%q terminal:%q}, stored terminal type = %q", task.Stage, task.Terminal, stored.Type)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, id := terminalReady(t)
			tc.run(t, svc, id)
		})
	}
}
