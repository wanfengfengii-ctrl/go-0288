package api_test

import (
	"bytes"
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

func TestModel_TerminalDecisionRequiresEvidenceClosure(t *testing.T) {
	tests := []struct {
		name       string
		decision   domain.TerminalType
		wantStatus int
		wantCode   string
		wantSaved  bool
	}{
		{name: "accept is rejected before pouring and integrity closure", decision: domain.TerminalAccept, wantStatus: http.StatusUnprocessableEntity, wantCode: string(domain.CodeGenerationConflict)},
		{name: "quarantine retains its early terminal barrier", decision: domain.TerminalQuarantine, wantStatus: http.StatusOK, wantSaved: true},
		{name: "cancel retains its early terminal barrier", decision: domain.TerminalCancel, wantStatus: http.StatusOK, wantSaved: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "piles.db"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = st.Close() })
			svc := service.New(st)
			h := api.NewServer(st, domain.Services{
				Design: svc, Trace: svc, Material: svc, Evidence: svc, Arbiter: svc, Store: st,
			}).Handler()

			post := func(path string, body any) *httptest.ResponseRecorder {
				t.Helper()
				payload, err := json.Marshal(body)
				if err != nil {
					t.Fatalf("marshal %s: %v", path, err)
				}
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				h.ServeHTTP(rec, req)
				return rec
			}
			mustPost := func(path string, body any, want int) *httptest.ResponseRecorder {
				t.Helper()
				rec := post(path, body)
				if rec.Code != want {
					t.Fatalf("POST %s status = %d, want %d; body=%s", path, rec.Code, want, rec.Body.String())
				}
				return rec
			}

			design := domain.CreateTaskRequest{
				Pier: "P1", PileNo: "1", Summary: "terminal closure acceptance",
				DesignDepth: 10000, Diameter: 1000,
				Layers: []domain.Layer{{Name: "soil", Start: 0, End: 5000}, {Name: "rock", Start: 5000, End: 10000}},
				Rebar:  []domain.RebarSegment{{Index: 0, Start: 0, End: 10000, Direction: domain.DirectionBottomUp}},
				Sonic: []domain.SonicTube{
					{ID: "S1", Start: 0, End: 10000, Neighbors: []string{"S2"}},
					{ID: "S2", Start: 0, End: 10000, Neighbors: []string{"S1"}},
				},
				Mud: domain.MudThresholds{
					SpecificGravityMin: 1_100_000, SpecificGravityMax: 1_300_000,
					ViscosityMin: 1_800_000, ViscosityMax: 2_500_000, SandContentMax: 40_000,
				},
				Cleaning: domain.CleaningThresholds{SedimentMax: 300, ApertureTolerance: 100},
				Pour:     domain.PourWindow{FirstPourVolume: 2000, ContinuousMaxGap: 100, MinEmbedment: 400, MaxEmbedment: 6000},
				Overpour: 500, LineAdjacency: [][]string{{"S1", "S2"}},
				Coring: domain.CoringRules{MinCoresPerAnomaly: 1, CoreDepthStep: 1000}, AgePeriod: 10, MaxRetries: 3,
			}
			created := mustPost("/v1/piles", design, http.StatusCreated)
			var createResult struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(created.Body.Bytes(), &createResult); err != nil || createResult.ID == "" {
				t.Fatalf("decode created pile: id=%q err=%v body=%s", createResult.ID, err, created.Body.String())
			}
			base := "/v1/piles/" + createResult.ID
			mustPost(base+"/lock", struct{}{}, http.StatusOK)
			mustPost(base+"/borehole-checks", domain.BoreholeRequest{
				FinalDepth: 10000,
				Aperture:   []domain.PointSample{{Depth: 0, Value: 1000}, {Depth: 5000, Value: 1000}, {Depth: 10000, Value: 1000}},
				Sediment:   []domain.PointSample{{Depth: 0, Value: 10}, {Depth: 5000, Value: 20}, {Depth: 10000, Value: 30}},
			}, http.StatusOK)
			mustPost(base+"/cleaning-acceptance", domain.CleaningRequest{Mud: []domain.MudSample{
				{Depth: 0, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
				{Depth: 5000, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
				{Depth: 10000, SpecificGravity: 1_200_000, Viscosity: 2_000_000, SandContent: 20_000},
			}}, http.StatusOK)
			mustPost(base+"/cages", domain.CagesRequest{Rebar: design.Rebar, Sonic: design.Sonic}, http.StatusOK)
			mustPost(base+"/conduits", domain.ConduitsRequest{
				HoleDepth: 10000,
				Segments: []domain.ConduitSegment{
					{Index: 0, Length: 5000, Direction: domain.DirectionBottomUp, Watertight: true},
					{Index: 1, Length: 5000, Direction: domain.DirectionBottomUp, Watertight: true},
				},
			}, http.StatusOK)
			mustPost(base+"/reviews", domain.ReviewRequest{ReviewerID: "reviewer-a", Qualified: true, Approve: true}, http.StatusOK)
			mustPost(base+"/reviews", domain.ReviewRequest{ReviewerID: "reviewer-b", Qualified: true, Approve: true}, http.StatusOK)

			decision := post(base+"/terminal-decisions", domain.DecisionRequest{ReviewerID: "reviewer-a", Type: tc.decision, Basis: "dual review"})
			if decision.Code != tc.wantStatus {
				t.Fatalf("terminal status = %d, want %d; body=%s", decision.Code, tc.wantStatus, decision.Body.String())
			}
			if tc.wantCode != "" {
				var failure struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(decision.Body.Bytes(), &failure); err != nil || failure.Code != tc.wantCode {
					t.Fatalf("terminal rejection code = %q, want %q; err=%v body=%s", failure.Code, tc.wantCode, err, decision.Body.String())
				}
			}

			terminal := httptest.NewRecorder()
			h.ServeHTTP(terminal, httptest.NewRequest(http.MethodGet, base+"/terminal", nil))
			if !tc.wantSaved {
				if terminal.Code != http.StatusNotFound {
					t.Fatalf("rejected acceptance created terminal: status=%d body=%s", terminal.Code, terminal.Body.String())
				}
				task := httptest.NewRecorder()
				h.ServeHTTP(task, httptest.NewRequest(http.MethodGet, base, nil))
				var state domain.PileTask
				if task.Code != http.StatusOK || json.Unmarshal(task.Body.Bytes(), &state) != nil || state.Stage != domain.StageConduitsAccepted {
					t.Fatalf("rejected acceptance changed task: status=%d stage=%q body=%s", task.Code, state.Stage, task.Body.String())
				}
				return
			}

			var saved domain.TerminalRecord
			if terminal.Code != http.StatusOK || json.Unmarshal(terminal.Body.Bytes(), &saved) != nil || saved.Type != tc.decision || saved.CredentialNo == "" {
				t.Fatalf("saved terminal = %+v, status=%d body=%s", saved, terminal.Code, terminal.Body.String())
			}
			repeat := post(base+"/terminal-decisions", domain.DecisionRequest{ReviewerID: "reviewer-b", Type: domain.TerminalAccept, Basis: "must lose"})
			if repeat.Code != http.StatusConflict {
				t.Fatalf("second terminal status = %d, want 409; body=%s", repeat.Code, repeat.Body.String())
			}
			var existing struct {
				Code     string                `json:"code"`
				Terminal domain.TerminalRecord `json:"terminal"`
			}
			if err := json.Unmarshal(repeat.Body.Bytes(), &existing); err != nil || existing.Code != string(domain.CodeTerminalAlreadyDecided) || existing.Terminal.Type != tc.decision || existing.Terminal.CredentialNo != saved.CredentialNo {
				t.Fatalf("second terminal did not return existing decision: %+v err=%v body=%s", existing, err, repeat.Body.String())
			}
		})
	}
}
