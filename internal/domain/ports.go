package domain

import "context"

// DesignCatalog validates and locks the design summary, depth segments,
// construction layout, material batches, fixed-point thresholds, device
// requirements, line-adjacency table and re-inspection rules into an immutable
// version snapshot (component 1 of the traceability matrix).
type DesignCatalog interface {
	// CreateTask creates an unlocked pile task from a full design baseline and
	// returns its identifier.
	CreateTask(ctx context.Context, req CreateTaskRequest) (PileID, error)
	// Lock validates and freezes the design snapshot, rejecting any mismatch or
	// a second lock attempt.
	Lock(ctx context.Context, id PileID) (DesignSnapshot, error)
	// Snapshot returns the locked design snapshot.
	Snapshot(ctx context.Context, id PileID) (DesignSnapshot, error)
	// Task returns the current task state.
	Task(ctx context.Context, id PileID) (PileTask, error)
}

// TaskTrace implements the flow state machine and the append-only depth trace,
// maintaining logical time, cumulative volume, theoretical/measured level,
// conduit prefix, embedment, overpour height and restart recovery (component 2).
type TaskTrace interface {
	// Borehole validates and records final-depth, aperture and sediment samples.
	Borehole(ctx context.Context, id PileID, req BoreholeRequest) error
	// AcceptCleaning validates and records slurry samples and advances to the
	// cleaning-accepted stage.
	AcceptCleaning(ctx context.Context, id PileID, req CleaningRequest) error
	// Cages validates and records rebar and acoustic-tube coverage.
	Cages(ctx context.Context, id PileID, req CagesRequest) error
	// Conduits validates and records the conduit assembly and water tightness.
	Conduits(ctx context.Context, id PileID, req ConduitsRequest) error
	// StartPour performs the first-pour base sealing atomically.
	StartPour(ctx context.Context, id PileID, req StartPourRequest) error
	// PourEntry performs a continuous-pour increment.
	PourEntry(ctx context.Context, id PileID, req PourRequest) error
	// LevelReading records a concrete level re-measurement.
	LevelReading(ctx context.Context, id PileID, req LevelRequest) error
	// RemoveSegments removes trailing active conduit segments.
	RemoveSegments(ctx context.Context, id PileID, req RemoveRequest) error
	// FinishPour records the pile-top finishing and closes the pour.
	FinishPour(ctx context.Context, id PileID, req FinishRequest) error
	// Trace returns the append-only pour trace in sequence order.
	Trace(ctx context.Context, id PileID) ([]PourTraceEntry, error)
}

// MaterialLease manages integer-litre batch pools with reserve/deduct
// transactions, and time-bounded mutual-exclusion leases for devices with
// renew, release and deterministic expiry collection (component 3).
type MaterialLease interface {
	// CreateBatch registers a concrete batch.
	CreateBatch(ctx context.Context, batch ConcreteBatch) error
	// Batch returns a batch's conservation state.
	Batch(ctx context.Context, batchID string) (ConcreteBatch, error)
	// Leases returns the leases held by a pile.
	Leases(ctx context.Context, id PileID) ([]DeviceLease, error)
	// Retry replays a failed device call with a scripted outcome.
	Retry(ctx context.Context, id PileID, callID string, req RetryRequest) error
}

// EvidenceRecorder validates borehole and mud samples, rebar/sonic-tube
// coverage, conduit assembly and water tightness, orchestrates device calls and
// records immutable first-pour/continuous-pour/segment-removal evidence
// (component 4).
type EvidenceRecorder interface {
	// Evidence returns the immutable evidence chain in deterministic order.
	Evidence(ctx context.Context, id PileID) ([]InspectionEvidence, error)
}

// IntegrityArbiter maintains inspection generations and the original evidence
// chain, deterministically expands the re-inspection set, isolates late
// receipts, verifies dual-reviewer qualification and arbitrates the single
// terminal decision via a single-writer barrier (component 5).
type IntegrityArbiter interface {
	// Integrity registers acoustic integrity results and expands the re-inspection
	// set, creating a new inspection generation when required.
	Integrity(ctx context.Context, id PileID, req IntegrityRequest) error
	// NewGeneration creates a fresh inspection generation.
	NewGeneration(ctx context.Context, id PileID) error
	// CoreResult records core-sampling results for the current generation.
	CoreResult(ctx context.Context, id PileID, req CoreRequest) error
	// Review records one independent dual-review decision.
	Review(ctx context.Context, id PileID, req ReviewRequest) error
	// Terminate competes for the unique terminal decision.
	Terminate(ctx context.Context, id PileID, req DecisionRequest) (TerminalRecord, error)
	// Terminal returns the existing terminal record, if any.
	Terminal(ctx context.Context, id PileID) (TerminalRecord, bool, error)
}

// Store is the persistence boundary (SQLite): transactions, unique/foreign/
// check constraints, optimistic versions, migrations and restart recovery
// (component 6's persistence half).
type Store interface {
	// Ping reports whether the persistence layer is reachable.
	Ping(ctx context.Context) error
	// Close releases the underlying connection.
	Close() error
}

// Services aggregates the six component interfaces. It is the seam the HTTP
// API and the executable entry point depend on. The Store is always wired so the
// health endpoint can report persistence status.
type Services struct {
	Design   DesignCatalog
	Trace    TaskTrace
	Material MaterialLease
	Evidence EvidenceRecorder
	Arbiter  IntegrityArbiter
	Store    Store
}
