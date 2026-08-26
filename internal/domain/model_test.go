package domain

import "testing"

func TestConcreteBatchReserve(t *testing.T) {
	b := &ConcreteBatch{ID: "B1", Initial: 1000}
	if b.Available() != 1000 {
		t.Fatalf("available = %d, want 1000", b.Available())
	}
	if err := b.Reserve(400); err != nil {
		t.Fatalf("Reserve(400) err = %v", err)
	}
	if b.Available() != 600 {
		t.Fatalf("available after reserve = %d, want 600", b.Available())
	}
	if err := b.Reserve(600); err != nil {
		t.Fatalf("Reserve(600) err = %v", err)
	}
	if b.Available() != 0 {
		t.Fatalf("available = %d, want 0", b.Available())
	}
	if err := b.Reserve(1); !IsCode(err, CodeConcreteInsufficient) {
		t.Fatalf("over-reserve err = %v, want CONCRETE_INSUFFICIENT", err)
	}
	if err := b.Reserve(-1); !IsCode(err, CodeConcreteInsufficient) {
		t.Fatalf("negative reserve err = %v, want CONCRETE_INSUFFICIENT", err)
	}
}

func TestValidateTransition(t *testing.T) {
	base := PourTraceEntry{Time: 10, TotalLitres: 100, TheoryLevel: 200}
	t.Run("ok", func(t *testing.T) {
		next := PourTraceEntry{Time: 11, TotalLitres: 150, TheoryLevel: 210}
		if err := ValidateTransition(base, next); err != nil {
			t.Fatalf("unexpected err %v", err)
		}
	})
	t.Run("time must increase", func(t *testing.T) {
		next := PourTraceEntry{Time: 10, TotalLitres: 150, TheoryLevel: 210}
		if err := ValidateTransition(base, next); !IsCode(err, CodePourInterrupted) {
			t.Fatalf("err = %v, want POUR_INTERRUPTED", err)
		}
	})
	t.Run("volume cannot regress", func(t *testing.T) {
		next := PourTraceEntry{Time: 11, TotalLitres: 99, TheoryLevel: 210}
		if err := ValidateTransition(base, next); !IsCode(err, CodeConcreteInsufficient) {
			t.Fatalf("err = %v, want CONCRETE_INSUFFICIENT", err)
		}
	})
	t.Run("level cannot regress", func(t *testing.T) {
		next := PourTraceEntry{Time: 11, TotalLitres: 150, TheoryLevel: 199}
		if err := ValidateTransition(base, next); !IsCode(err, CodePourInterrupted) {
			t.Fatalf("err = %v, want POUR_INTERRUPTED", err)
		}
	})
}

func TestDepthRangeValid(t *testing.T) {
	cases := []struct {
		r    DepthRange
		want bool
	}{
		{DepthRange{Start: 0, End: 10}, true},
		{DepthRange{Start: 5, End: 5}, true},
		{DepthRange{Start: -1, End: 5}, false},
		{DepthRange{Start: 5, End: 4}, false},
	}
	for _, c := range cases {
		if got := c.r.Valid(); got != c.want {
			t.Fatalf("Valid(%+v) = %v, want %v", c.r, got, c.want)
		}
	}
}
