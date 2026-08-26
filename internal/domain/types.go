// Package domain holds the stable value types, data model, error catalogue and
// component interfaces that bind the pile-pouring quality-closure system
// together. Every type here maps to an entity or rule in PROJECT_SPEC.
package domain

// LogicalTime is a monotonically increasing integer timestamp. All ordering,
// lease expiry and "next retry" decisions use LogicalTime; wall-clock time
// never participates in adjudication.
type LogicalTime int64

// PileID identifies a single pile task.
type PileID string

// Generation numbers a design snapshot or an inspection generation. It is
// monotonically increasing and never reused.
type Generation int64

// LeaseToken uniquely identifies a granted device lease.
type LeaseToken string

// Direction describes assembly or coverage orientation.
type Direction int

const (
	// DirectionTopDown denotes top-to-bottom assembly or coverage.
	DirectionTopDown Direction = iota
	// DirectionBottomUp denotes bottom-to-top assembly or coverage.
	DirectionBottomUp
)

// String returns a stable name for the direction.
func (d Direction) String() string {
	switch d {
	case DirectionBottomUp:
		return "bottom_up"
	default:
		return "top_down"
	}
}

// Stage is the flow state-machine stage of a pile task.
type Stage string

const (
	StageCreated          Stage = "created"
	StageLocked           Stage = "locked"
	StageBoreholeChecked  Stage = "borehole_checked"
	StageCleaningAccepted Stage = "cleaning_accepted"
	StageCagesAccepted    Stage = "cages_accepted"
	StageConduitsAccepted Stage = "conduits_accepted"
	StagePoured           Stage = "poured"
	StageFinished         Stage = "finished"
	StageInspected        Stage = "inspected"
	StageTerminal         Stage = "terminal"
)
