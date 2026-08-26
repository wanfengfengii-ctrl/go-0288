package domain

import (
	"errors"
	"testing"
)

func TestErrorCodesAreStable(t *testing.T) {
	want := map[ErrorCode]string{
		CodeDesignMismatch:         "DESIGN_MISMATCH",
		CodeBoreholeConflict:       "BOREHOLE_CONFLICT",
		CodeSampleMissing:          "SAMPLE_MISSING",
		CodeRebarLayoutInvalid:     "REBAR_LAYOUT_INVALID",
		CodeConduitDuplicate:       "CONDUIT_DUPLICATE",
		CodeConcreteInsufficient:   "CONCRETE_INSUFFICIENT",
		CodeLeaseConflict:          "LEASE_CONFLICT",
		CodeFirstPourInsufficient:  "FIRST_POUR_INSUFFICIENT",
		CodeEmbedmentOutOfRange:    "EMBEDMENT_OUT_OF_RANGE",
		CodePourInterrupted:        "POUR_INTERRUPTED",
		CodeFixedPointOverflow:     "FIXED_POINT_OVERFLOW",
		CodeDeviceRejected:         "DEVICE_REJECTED",
		CodeDeviceTimeout:          "DEVICE_TIMEOUT",
		CodeGenerationConflict:     "GENERATION_CONFLICT",
		CodeIdempotencyConflict:    "IDEMPOTENCY_CONFLICT",
		CodeTerminalAlreadyDecided: "TERMINAL_ALREADY_DECIDED",
	}
	if len(want) != 16 {
		t.Fatalf("expected 16 stable error codes, got %d", len(want))
	}
	for code, s := range want {
		if string(code) != s {
			t.Fatalf("code %q != %q", code, s)
		}
	}
}

func TestNormalizeSortsReasons(t *testing.T) {
	e := &Error{
		Code:    CodeBoreholeConflict,
		Message: "multi-cause",
		Reasons: []Reason{
			{Pier: "B", Pile: "2", DepthStart: 100, Code: CodeSampleMissing},
			{Pier: "A", Pile: "1", DepthStart: 200, Code: CodeBoreholeConflict},
			{Pier: "A", Pile: "1", DepthStart: 100, Code: CodeSampleMissing},
		},
	}
	got := e.Normalize().ReasonCodes()
	want := []ErrorCode{CodeSampleMissing, CodeBoreholeConflict, CodeSampleMissing}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reason[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// Normalization must be idempotent.
	again := e.Normalize().ReasonCodes()
	for i := range want {
		if again[i] != want[i] {
			t.Fatalf("idempotent reason[%d] = %q, want %q", i, again[i], want[i])
		}
	}
}

func TestIsCode(t *testing.T) {
	if !IsCode(NewError(CodeConduitDuplicate, "dup"), CodeConduitDuplicate) {
		t.Fatal("IsCode should match the code")
	}
	if IsCode(NewError(CodeConduitDuplicate, "dup"), CodeDesignMismatch) {
		t.Fatal("IsCode should not match a different code")
	}
	if IsCode(errors.New("plain"), CodeConduitDuplicate) {
		t.Fatal("IsCode should not match a non-domain error")
	}
	if IsCode(nil, CodeConduitDuplicate) {
		t.Fatal("IsCode should not match nil")
	}
}
