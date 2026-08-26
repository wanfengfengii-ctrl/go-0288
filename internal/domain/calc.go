package domain

import (
	"deep-pile-pour-integrity-closure/internal/fixed"
)

// PiFixed is the fixed-point representation of pi using fixed.RatioScale as the
// denominator. It is the single source of truth for circular cross-section
// geometry so that the same rounding is applied in every adjudication.
const PiFixed int64 = 3_141_593 // pi * 1_000_000

// mm3PerLitre is the number of cubic millimetres in one litre. It is used to
// convert between integer litres (volume) and integer millimetres (height).
const mm3PerLitre int64 = 1_000_000

// CrossArea returns the circular cross-sectional area of a pile of the given
// diameter, expressed in square millimetres. The value is rounded half away
// from zero and every intermediate product is overflow-checked.
func CrossArea(diameter int64) (int64, error) {
	if diameter <= 0 {
		return 0, NewError(CodeDesignMismatch, "diameter must be positive")
	}
	d2, err := fixed.Mul(diameter, diameter)
	if err != nil {
		return 0, NewError(CodeFixedPointOverflow, "diameter squared overflows")
	}
	num, err := fixed.Mul(PiFixed, d2)
	if err != nil {
		return 0, NewError(CodeFixedPointOverflow, "area numerator overflows")
	}
	area, err := fixed.RoundDiv(num, 4*fixed.RatioScale)
	if err != nil {
		return 0, NewError(CodeFixedPointOverflow, "area division failed")
	}
	return area, nil
}

// ConcreteHeight returns the height of concrete (in millimetres) produced by a
// given cumulative volume of litres poured into a constant cross-section.
func ConcreteHeight(totalLitres, area int64) (int64, error) {
	if area <= 0 {
		return 0, NewError(CodeDesignMismatch, "cross-section area must be positive")
	}
	if totalLitres < 0 {
		return 0, NewError(CodeConcreteInsufficient, "cumulative volume cannot be negative")
	}
	h, err := fixed.MulDiv(totalLitres, mm3PerLitre, area)
	if err != nil {
		return 0, NewError(CodeFixedPointOverflow, "concrete height overflows")
	}
	return h, nil
}

// TheoreticalLevel computes the concrete surface depth below the pile top (mm)
// for a cumulative volume. A negative value denotes overpour above the design
// top. The overpour height (mm) is returned as a non-negative magnitude.
func TheoreticalLevel(designDepth, totalLitres, area int64) (level int64, overpour int64, err error) {
	h, err := ConcreteHeight(totalLitres, area)
	if err != nil {
		return 0, 0, err
	}
	level, err = fixed.Sub(designDepth, h)
	if err != nil {
		return 0, 0, NewError(CodeFixedPointOverflow, "theoretical level overflows")
	}
	if level < 0 {
		overpour = -level
	}
	return level, overpour, nil
}

// Embedment returns the distance (mm) by which the conduit bottom sits below
// the concrete surface, given the conduit bottom depth and the concrete level
// (both measured as depth below pile top). A negative result means the conduit
// bottom is above the concrete surface.
func Embedment(conduitBottomDepth, concreteLevel int64) int64 {
	return conduitBottomDepth - concreteLevel
}

// VolumeFromHeight returns the volume in litres occupied by a concrete column
// of the given height (mm) and cross-sectional area (mm^2).
func VolumeFromHeight(height, area int64) (int64, error) {
	if height < 0 {
		return 0, NewError(CodeConcreteInsufficient, "height cannot be negative")
	}
	if area <= 0 {
		return 0, NewError(CodeDesignMismatch, "cross-section area must be positive")
	}
	v, err := fixed.MulDiv(height, area, mm3PerLitre)
	if err != nil {
		return 0, NewError(CodeFixedPointOverflow, "volume conversion overflows")
	}
	return v, nil
}
