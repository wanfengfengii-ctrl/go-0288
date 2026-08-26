package domain

import (
	"fmt"
	"sort"
	"strings"
)

// ErrorCode is a stable, machine-readable rejection code. These codes are part
// of the public API contract and must remain identical across runs and
// restarts so black-box tests can assert them deterministically.
type ErrorCode string

const (
	CodeDesignMismatch         ErrorCode = "DESIGN_MISMATCH"
	CodeBoreholeConflict       ErrorCode = "BOREHOLE_CONFLICT"
	CodeSampleMissing          ErrorCode = "SAMPLE_MISSING"
	CodeRebarLayoutInvalid     ErrorCode = "REBAR_LAYOUT_INVALID"
	CodeConduitDuplicate       ErrorCode = "CONDUIT_DUPLICATE"
	CodeConcreteInsufficient   ErrorCode = "CONCRETE_INSUFFICIENT"
	CodeLeaseConflict          ErrorCode = "LEASE_CONFLICT"
	CodeFirstPourInsufficient  ErrorCode = "FIRST_POUR_INSUFFICIENT"
	CodeEmbedmentOutOfRange    ErrorCode = "EMBEDMENT_OUT_OF_RANGE"
	CodePourInterrupted        ErrorCode = "POUR_INTERRUPTED"
	CodeFixedPointOverflow     ErrorCode = "FIXED_POINT_OVERFLOW"
	CodeDeviceRejected         ErrorCode = "DEVICE_REJECTED"
	CodeDeviceTimeout          ErrorCode = "DEVICE_TIMEOUT"
	CodeGenerationConflict     ErrorCode = "GENERATION_CONFLICT"
	CodeIdempotencyConflict    ErrorCode = "IDEMPOTENCY_CONFLICT"
	CodeTerminalAlreadyDecided ErrorCode = "TERMINAL_ALREADY_DECIDED"
)

// Reason describes a single cause within a multi-cause rejection. It carries
// the sortable dimensions documented in the failure boundary (pier, pile,
// depth interval, logical time, conduit segment, line) so that normalisation is
// fully deterministic.
type Reason struct {
	Pier       string
	Pile       string
	DepthStart int64 // millimetres
	DepthEnd   int64 // millimetres
	Time       LogicalTime
	Segment    int
	Line       string
	Code       ErrorCode
	Detail     string
}

// SortKey renders a stable, deterministic ordering key for the reason.
func (r Reason) SortKey() string {
	return fmt.Sprintf("%s|%s|%020d|%020d|%020d|%010d|%s|%s",
		r.Pier, r.Pile, r.DepthStart, r.DepthEnd, int64(r.Time),
		r.Segment, r.Line, r.Code)
}

// Error is a domain rejection carrying a stable code and, when multiple causes
// apply, a deterministically ordered list of reasons.
type Error struct {
	Code    ErrorCode
	Message string
	Reasons []Reason
}

// Error implements the error interface, rendering the code and message.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewError constructs a single-cause rejection.
func NewError(code ErrorCode, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

// Normalize sorts the reasons by their stable key and returns the receiver,
// so the same input produces the same reason order across runs and restarts.
func (e *Error) Normalize() *Error {
	if e == nil {
		return nil
	}
	sort.SliceStable(e.Reasons, func(i, j int) bool {
		return e.Reasons[i].SortKey() < e.Reasons[j].SortKey()
	})
	return e
}

// ReasonCodes returns the codes of the normalised reasons in order.
func (e *Error) ReasonCodes() []ErrorCode {
	if e == nil {
		return nil
	}
	out := make([]ErrorCode, 0, len(e.Reasons))
	for _, r := range e.Reasons {
		out = append(out, r.Code)
	}
	return out
}

// IsCode reports whether the error is a *Error with the given code.
func IsCode(err error, code ErrorCode) bool {
	de, ok := err.(*Error)
	return ok && de != nil && de.Code == code
}

// MultiCode joins several codes into a single stable string for comparison.
func MultiCode(codes ...ErrorCode) string {
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = string(c)
	}
	return strings.Join(parts, ",")
}
