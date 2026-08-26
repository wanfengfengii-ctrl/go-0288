package fixed

import (
	"errors"
	"math"
	"testing"
)

func TestAdd(t *testing.T) {
	cases := []struct {
		name    string
		a, b    int64
		want    int64
		wantErr error
	}{
		{"positive", 1, 2, 3, nil},
		{"zero", math.MaxInt64, 0, math.MaxInt64, nil},
		{"negative", -5, -3, -8, nil},
		{"overflow positive", math.MaxInt64, 1, 0, ErrOverflow},
		{"overflow negative", math.MinInt64, -1, 0, ErrOverflow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Add(c.a, c.b)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("Add(%d,%d) err = %v, want %v", c.a, c.b, err, c.wantErr)
			}
			if err == nil && got != c.want {
				t.Fatalf("Add(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestSub(t *testing.T) {
	cases := []struct {
		name    string
		a, b    int64
		want    int64
		wantErr error
	}{
		{"positive", 5, 3, 2, nil},
		{"negative result", 3, 5, -2, nil},
		{"overflow negative", math.MinInt64, 1, 0, ErrOverflow},
		{"overflow positive", math.MaxInt64, -1, 0, ErrOverflow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Sub(c.a, c.b)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("Sub(%d,%d) err = %v, want %v", c.a, c.b, err, c.wantErr)
			}
			if err == nil && got != c.want {
				t.Fatalf("Sub(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestMul(t *testing.T) {
	cases := []struct {
		name    string
		a, b    int64
		want    int64
		wantErr error
	}{
		{"positive", 2, 3, 6, nil},
		{"zero", 0, math.MaxInt64, 0, nil},
		{"overflow", math.MaxInt64, 2, 0, ErrOverflow},
		{"min times minus one", math.MinInt64, -1, 0, ErrOverflow},
		{"minus one times min", -1, math.MinInt64, 0, ErrOverflow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Mul(c.a, c.b)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("Mul(%d,%d) err = %v, want %v", c.a, c.b, err, c.wantErr)
			}
			if err == nil && got != c.want {
				t.Fatalf("Mul(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestRoundDiv(t *testing.T) {
	cases := []struct {
		name    string
		a, b    int64
		want    int64
		wantErr error
	}{
		{"exact", 4, 2, 2, nil},
		{"half up", 5, 2, 3, nil},
		{"half down", 7, 3, 2, nil},
		{"negative half away", -5, 2, -3, nil},
		{"negative divisor", 5, -2, -3, nil},
		{"divide by zero", 1, 0, 0, ErrDivideByZero},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RoundDiv(c.a, c.b)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("RoundDiv(%d,%d) err = %v, want %v", c.a, c.b, err, c.wantErr)
			}
			if err == nil && got != c.want {
				t.Fatalf("RoundDiv(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestMulDiv(t *testing.T) {
	cases := []struct {
		name    string
		a, b, c int64
		want    int64
		wantErr error
	}{
		{"exact", 6, 7, 2, 21, nil},
		{"rounding", 1, 5, 2, 3, nil},
		{"intermediate overflow", math.MaxInt64, 2, 1, 0, ErrOverflow},
		{"divide by zero", 1, 1, 0, 0, ErrDivideByZero},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := MulDiv(c.a, c.b, c.c)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("MulDiv(%d,%d,%d) err = %v, want %v", c.a, c.b, c.c, err, c.wantErr)
			}
			if err == nil && got != c.want {
				t.Fatalf("MulDiv(%d,%d,%d) = %d, want %d", c.a, c.b, c.c, got, c.want)
			}
		})
	}
}
