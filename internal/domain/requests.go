package domain

import "sort"

// CleaningThresholds holds the locked borehole-cleaning acceptance thresholds.
type CleaningThresholds struct {
	SedimentMax       int64 // mm, maximum permitted sediment thickness
	ApertureTolerance int64 // mm, aperture reading tolerance around the design diameter
}

// CreateTaskRequest carries the full design and rules baseline for a new pile.
type CreateTaskRequest struct {
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
	Overpour      int64
	LineAdjacency [][]string
	Coring        CoringRules
	AgePeriod     LogicalTime
	MaxRetries    int
}

// Snapshot builds the immutable design snapshot from the request. The
// generation is supplied by the caller (the catalogue assigns it on lock).
func (r CreateTaskRequest) Snapshot(gen Generation) DesignSnapshot {
	return DesignSnapshot{
		Generation:    gen,
		Pier:          r.Pier,
		PileNo:        r.PileNo,
		Summary:       r.Summary,
		DesignDepth:   r.DesignDepth,
		Diameter:      r.Diameter,
		Layers:        r.Layers,
		Rebar:         r.Rebar,
		Sonic:         r.Sonic,
		Mud:           r.Mud,
		Cleaning:      r.Cleaning,
		Pour:          r.Pour,
		Overpour:      r.Overpour,
		LineAdjacency: r.LineAdjacency,
		Coring:        r.Coring,
		AgePeriod:     r.AgePeriod,
		MaxRetries:    r.MaxRetries,
	}
}

// Validate performs the full design-and-rules pre-lock validation, returning a
// normalised multi-cause error on failure.
func (r CreateTaskRequest) Validate() *Error {
	return ValidateSnapshot(r.Snapshot(0))
}

// validateAdjacency checks that the line-adjacency table references only known
// acoustic tubes and is symmetric.
func validateAdjacency(adj [][]string, sonic []SonicTube) *Error {
	ids := make(map[string]bool, len(sonic))
	for _, s := range sonic {
		ids[s.ID] = true
	}
	for _, pair := range adj {
		if len(pair) != 2 {
			return NewError(CodeDesignMismatch, "line adjacency pairs must have two tubes")
		}
		if !ids[pair[0]] || !ids[pair[1]] {
			return NewError(CodeDesignMismatch, "line adjacency references an unknown tube")
		}
	}
	return nil
}

// PointSample is a single depth-pointed measurement (aperture or sediment).
type PointSample struct {
	Depth int64 // mm
	Value int64 // mm
}

// MudSample is a single depth-pointed slurry measurement (fixed-point values).
type MudSample struct {
	Depth           int64 // mm
	SpecificGravity int64 // fixed
	Viscosity       int64 // fixed
	SandContent     int64 // fixed percentage
}

// BoreholeRequest carries the final-depth, aperture and sediment measurements.
type BoreholeRequest struct {
	FinalDepth int64
	Aperture   []PointSample
	Sediment   []PointSample
}

// CleaningRequest carries the slurry acceptance samples.
type CleaningRequest struct {
	Mud []MudSample
}

// CagesRequest carries the as-built rebar and acoustic-tube coverage.
type CagesRequest struct {
	Rebar []RebarSegment
	Sonic []SonicTube
}

// ConduitsRequest carries the ordered, water-tight conduit assembly.
type ConduitsRequest struct {
	Segments  []ConduitSegment
	HoleDepth int64 // measured hole depth used for compatibility
}

// DeviceRequest captures the device-lease acquisition parameters for an
// operation that requires a physical resource. LeaseStart is the operation's
// logical time; LeaseEnd is the requested end of the lease.
type DeviceRequest struct {
	DeviceType DeviceType
	ResourceID string
	LeaseEnd   LogicalTime
}

// PourRequest carries a continuous-pour increment with its batch deduction,
// device lease and logical time.
type PourRequest struct {
	OperationID   string
	Time          LogicalTime
	BatchID       string
	Litres        int64
	Device        DeviceRequest
	MeasuredLevel int64 // optional measured concrete level (0 = absent)
}

// StartPourRequest carries the first-pour (base sealing) details.
type StartPourRequest struct {
	OperationID string
	Time        LogicalTime
	BatchID     string
	Litres      int64
	Device      DeviceRequest
}

// LevelRequest carries a concrete level re-measurement.
type LevelRequest struct {
	OperationID   string
	Time          LogicalTime
	MeasuredLevel int64
	Device        DeviceRequest
	DeviceOutcome string // "", "rejected", "timeout", "malformed"
}

// RemoveRequest carries a conduit segment removal (from the active top).
type RemoveRequest struct {
	OperationID string
	Time        LogicalTime
	Count       int // number of trailing active segments to remove
}

// FinishRequest marks the pour complete (pile top finishing).
type FinishRequest struct {
	OperationID string
	Time        LogicalTime
}

// RetryRequest replays a failed device call with a scripted outcome.
type RetryRequest struct {
	Time    LogicalTime
	Outcome string // "success" or a failure kind (rejected/timeout/malformed)
	Reading int64  // reading produced on success
}

// IntegrityRequest registers an acoustic-wave integrity result per line.
type IntegrityRequest struct {
	Generation Generation
	Lines      []LineResult
}

// LineResult is one acoustic line's outcome.
type LineResult struct {
	Line          string
	AnomalyRanges []DepthRange
	Anomalous     bool
}

// CoreRequest registers a core-sampling result.
type CoreRequest struct {
	Generation    Generation
	Ranges        []DepthRange
	Findings      []CoreFinding
	Device        DeviceRequest
	DeviceOutcome string // "", "rejected", "timeout", "malformed"
}

// CoreFinding is a single core-sample finding.
type CoreFinding struct {
	Range    DepthRange
	Defect   string
	Severity string
}

// ReviewRequest carries one independent dual-review decision.
type ReviewRequest struct {
	ReviewerID string
	Qualified  bool
	Approve    bool
}

// DecisionRequest competes for the terminal outcome.
type DecisionRequest struct {
	ReviewerID string
	Type       TerminalType
	Basis      string
}

// CheckPoints returns the deterministic sampling depths for a locked design: the
// pile top (0) and every stratum boundary up to the design depth.
func CheckPoints(layers []Layer, designDepth int64) []int64 {
	set := map[int64]bool{0: true, designDepth: true}
	for _, l := range layers {
		set[l.Start] = true
		set[l.End] = true
	}
	out := make([]int64, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
