// Package fixed provides overflow-checked integer arithmetic for the project's
// fixed-point quantities. All physical magnitudes in this domain are integers:
// lengths in millimetres, volumes in litres, and ratios/percentages as scaled
// integers. Floating-point arithmetic is forbidden in adjudication, so every
// conversion and combination fails loudly on overflow or division by zero
// rather than silently wrapping.
package fixed

import (
	"errors"
	"math"
)

// RatioScale is the denominator of fixed-point ratio and percentage values.
// A Ratio of 1_000_000 represents 1.0 (100%), and 15_000 represents 1.5%.
const RatioScale int64 = 1_000_000

// ErrOverflow reports an integer operation that would exceed the int64 range.
var ErrOverflow = errors.New("fixed: integer overflow")

// ErrDivideByZero reports a division by a zero denominator.
var ErrDivideByZero = errors.New("fixed: division by zero")

// Add returns a+b, or ErrOverflow if the result cannot be represented.
func Add(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, ErrOverflow
	}
	return a + b, nil
}

// Sub returns a-b, or ErrOverflow if the result cannot be represented.
func Sub(a, b int64) (int64, error) {
	if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
		return 0, ErrOverflow
	}
	return a - b, nil
}

// Mul returns a*b, or ErrOverflow if the result cannot be represented.
func Mul(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	// math.MinInt64 * -1 cannot be represented even though the wrapped product
	// would coincidentally equal a in the quotient check below.
	if (a == -1 && b == math.MinInt64) || (b == -1 && a == math.MinInt64) {
		return 0, ErrOverflow
	}
	r := a * b
	if r/b != a {
		return 0, ErrOverflow
	}
	return r, nil
}

// MulDiv returns (a*b)/c rounded half away from zero, checking for overflow in
// the intermediate product and for a zero divisor.
func MulDiv(a, b, c int64) (int64, error) {
	if c == 0 {
		return 0, ErrDivideByZero
	}
	p, err := Mul(a, b)
	if err != nil {
		return 0, err
	}
	return RoundDiv(p, c)
}

// RoundDiv returns a/b rounded to the nearest integer, ties rounded away from
// zero. It reports ErrDivideByZero when b is zero. Domain scales are positive,
// so the divisor is expected to be positive in practice.
func RoundDiv(a, b int64) (int64, error) {
	if b == 0 {
		return 0, ErrDivideByZero
	}
	q := a / b
	rem := a % b
	if rem == 0 {
		return q, nil
	}
	absB := b
	if absB < 0 {
		absB = -absB
	}
	absRem := rem
	if absRem < 0 {
		absRem = -absRem
	}
	// Round the magnitude up when 2*|rem| >= |b|, expressed as
	// |rem| >= |b| - |rem| to avoid overflowing the doubled remainder.
	if absRem >= absB-absRem {
		if (a > 0) == (b > 0) {
			q++
		} else {
			q--
		}
	}
	return q, nil
}
