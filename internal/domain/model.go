package domain

// This file defines the persistent data model. Each type corresponds to an
// entity in PROJECT_SPEC's "数据模型与持久化" section. Integer magnitudes use
// millimetres (length) and litres (volume); ratios use the fixed.RatioScale
// convention. Field sets are intentionally minimal and stable; the completing
// session fills in the remaining adjudication logic, not additional entities.

// Layer is a stratum depth segment measured in millimetres from the pile top.
type Layer struct {
	Name  string
	Start int64 // mm
	End   int64 // mm
}

// RebarSegment is one reinforcing-cage section with its splice direction.
type RebarSegment struct {
	Index     int
	Start     int64 // mm
	End       int64 // mm
	Direction Direction
}

// SonicTube is one acoustic detection tube with its line-neighbours.
type SonicTube struct {
	ID        string
	Start     int64 // mm
	End       int64 // mm
	Neighbors []string
}

// MudThresholds holds the locked slurry acceptance thresholds (fixed-point).
type MudThresholds struct {
	SpecificGravityMin int64
	SpecificGravityMax int64
	ViscosityMin       int64
	ViscosityMax       int64
	SandContentMax     int64 // percentage, fixed-point
}

// PourWindow holds the locked continuous-pour constraints.
type PourWindow struct {
	FirstPourVolume  int64       // litres required to seal the base
	ContinuousMaxGap LogicalTime // maximum gap between consecutive pours
	MinEmbedment     int64       // mm
	MaxEmbedment     int64       // mm
}

// CoringRules holds the locked core-sampling rules.
type CoringRules struct {
	MinCoresPerAnomaly int
	CoreDepthStep      int64 // mm
}

// DesignSnapshot is the immutable, locked design and rules baseline.
type DesignSnapshot struct {
	Generation    Generation
	Pier          string
	PileNo        string
	Summary       string
	DesignDepth   int64 // mm
	Diameter      int64 // mm
	Layers        []Layer
	Rebar         []RebarSegment
	Sonic         []SonicTube
	Mud           MudThresholds
	Cleaning      CleaningThresholds
	Pour          PourWindow
	Overpour      int64      // mm
	LineAdjacency [][]string // acoustic line adjacency table
	Coring        CoringRules
	AgePeriod     LogicalTime // curing age before integrity inspection
	MaxRetries    int         // maximum device retry attempts before a quality anomaly
}

// PileTask is the mutable task record (optimistically versioned).
type PileTask struct {
	ID            PileID
	Stage         Stage
	LockedVersion int64
	Generation    Generation
	LastTime      LogicalTime
	AgeDeadline   LogicalTime
	Version       int64
	Terminal      TerminalType
}

// EvidenceType enumerates the recorded evidence kinds.
type EvidenceType string

const (
	EvidenceBorehole  EvidenceType = "borehole"
	EvidenceMud       EvidenceType = "mud"
	EvidenceRebar     EvidenceType = "rebar"
	EvidenceSonic     EvidenceType = "sonic"
	EvidenceConduit   EvidenceType = "conduit"
	EvidencePour      EvidenceType = "pour"
	EvidenceIntegrity EvidenceType = "integrity"
	EvidenceCore      EvidenceType = "core"
)

// DepthRange is an inclusive depth interval in millimetres.
type DepthRange struct {
	Start int64 // mm
	End   int64 // mm
}

// Valid reports whether the range is well-formed (non-empty, non-inverted).
func (r DepthRange) Valid() bool {
	return r.Start >= 0 && r.End >= r.Start
}

// InspectionEvidence records one immutable measurement or observation.
type InspectionEvidence struct {
	Type          EvidenceType
	Range         DepthRange
	Value         int64
	DeviceCallID  string
	Time          LogicalTime
	Generation    Generation
	PayloadDigest string
	Valid         bool
}

// ConduitSegment is one conduit pipe section with its splice direction and
// water-tightness result.
type ConduitSegment struct {
	Index      int
	Length     int64 // mm
	Direction  Direction
	Watertight bool
}

// ConduitAssembly is the ordered conduit stack. ActivePrefix is the number of
// leading segments currently in use; it must always be a prefix of Segments.
type ConduitAssembly struct {
	Segments     []ConduitSegment
	ActivePrefix int
	BottomDepth  int64 // mm, bottom of the active prefix
}

// PourEventType enumerates the append-only trace event kinds.
type PourEventType string

const (
	PourFirst         PourEventType = "first"
	PourContinue      PourEventType = "continue"
	PourLevelReading  PourEventType = "level"
	PourRemoveSegment PourEventType = "remove"
	PourFinish        PourEventType = "finish"
)

// PourTraceEntry is one append-only trace record.
type PourTraceEntry struct {
	Seq           int64
	OperationID   string
	Time          LogicalTime
	EventType     PourEventType
	BatchLitres   int64 // litres deducted by this entry
	TotalLitres   int64 // cumulative litres
	TheoryLevel   int64 // mm, theoretical concrete level
	MeasuredLevel int64 // mm, measured concrete level
	ConduitPrefix int
	Embedment     int64 // mm
	Overpour      int64 // mm
}

// ValidateTransition enforces the append-only invariants between consecutive
// trace entries: strictly increasing logical time, and non-decreasing
// cumulative volume and theoretical level (domain rule 4).
func ValidateTransition(prev, next PourTraceEntry) error {
	if next.Time <= prev.Time {
		return NewError(CodePourInterrupted, "logical time must strictly increase")
	}
	if next.TotalLitres < prev.TotalLitres {
		return NewError(CodeConcreteInsufficient, "cumulative volume cannot regress")
	}
	if next.TheoryLevel < prev.TheoryLevel {
		return NewError(CodePourInterrupted, "theoretical level cannot regress")
	}
	return nil
}

// ConcreteBatch is an integer-litre batch pool.
type ConcreteBatch struct {
	ID       string
	Initial  int64 // litres
	Deducted int64 // litres
}

// Available returns the remaining litres in the batch.
func (b ConcreteBatch) Available() int64 {
	return b.Initial - b.Deducted
}

// Reserve atomically deducts litres, rejecting a balance shortfall. It maps to
// the CONCRETE_INSUFFICIENT failure boundary and domain rule 6.
func (b *ConcreteBatch) Reserve(litres int64) error {
	if litres < 0 {
		return NewError(CodeConcreteInsufficient, "cannot deduct a negative volume")
	}
	if litres > b.Available() {
		return NewError(CodeConcreteInsufficient, "insufficient concrete balance")
	}
	b.Deducted += litres
	return nil
}

// DeviceType enumerates the leaseable device kinds.
type DeviceType string

const (
	DeviceConcretePump DeviceType = "concrete_pump"
	DeviceSoundingLine DeviceType = "sounding_line"
	DeviceMudMeter     DeviceType = "mud_meter"
	DeviceSonicMeter   DeviceType = "sonic_meter"
	DeviceCoringRig    DeviceType = "coring_rig"
)

// LeaseStatus enumerates the lifecycle states of a device lease.
type LeaseStatus string

const (
	LeaseActive   LeaseStatus = "active"
	LeaseExpired  LeaseStatus = "expired"
	LeaseReleased LeaseStatus = "released"
)

// DeviceLease is a time-bounded mutual-exclusion lease on a device resource.
type DeviceLease struct {
	DeviceType DeviceType
	ResourceID string
	Holder     PileID
	Token      LeaseToken
	Start      LogicalTime
	End        LogicalTime
	Status     LeaseStatus
}

// RetryCall is a pending device invocation awaiting retry.
type RetryCall struct {
	ID          string
	Request     string
	Attempts    int
	NextRetry   LogicalTime
	FailureCode ErrorCode
}

// ReviewGeneration is one inspection generation and its derived re-inspection
// set.
type ReviewGeneration struct {
	Generation    Generation
	LineEvidence  []string
	AnomalyRanges []DepthRange
	ReinspectSet  []DepthRange
	Parent        Generation
	Conclusion    string
}

// IdempotencyRecord stores the outcome of a previously executed operation.
type IdempotencyRecord struct {
	OperationID   string
	RequestDigest string
	SavedResult   string
}

// ReviewerDecision is one independent dual-review decision.
type ReviewerDecision struct {
	ReviewerID    string
	Qualification string
	Approve       bool
	TerminalType  TerminalType
}

// TerminalType enumerates the three competing terminal outcomes.
type TerminalType string

const (
	TerminalAccept     TerminalType = "accept"
	TerminalQuarantine TerminalType = "quarantine"
	TerminalCancel     TerminalType = "cancel"
)

// TerminalRecord is the single, unique terminal credential for a pile.
type TerminalRecord struct {
	PileID       PileID
	Type         TerminalType
	CredentialNo string
	Basis        string
	Version      int64
}
