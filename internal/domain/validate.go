package domain

import "sort"

// ValidateLayers checks that a stratum list is a contiguous, non-overlapping
// partition of [0, designDepth]. Each layer must have a positive extent and
// layers must be sorted by increasing depth with no gaps.
func ValidateLayers(layers []Layer, designDepth int64) *Error {
	if len(layers) == 0 {
		return NewError(CodeDesignMismatch, "at least one stratum layer is required")
	}
	sorted := append([]Layer(nil), layers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })

	var reasons []Reason
	cursor := int64(0)
	for _, l := range sorted {
		if l.End <= l.Start {
			reasons = append(reasons, Reason{
				DepthStart: l.Start, DepthEnd: l.End, Code: CodeDesignMismatch,
				Detail: "layer must have positive extent",
			})
			continue
		}
		if l.Start != cursor {
			reasons = append(reasons, Reason{
				DepthStart: cursor, DepthEnd: l.Start, Code: CodeDesignMismatch,
				Detail: "stratum layers have a gap or overlap",
			})
		}
		cursor = l.End
	}
	if len(reasons) == 0 && cursor != designDepth {
		reasons = append(reasons, Reason{
			DepthStart: cursor, DepthEnd: designDepth, Code: CodeDesignMismatch,
			Detail: "stratum layers do not reach the design depth",
		})
	}
	if len(reasons) == 0 {
		return nil
	}
	return (&Error{Code: CodeDesignMismatch, Message: "invalid stratum layers", Reasons: reasons}).Normalize()
}

// validateSegments checks a set of depth segments for continuous, non-overlapping
// coverage of [0, depth] with consistent direction. The callback yields each
// segment's start, end and direction in ascending start order.
func validateSegments(segments []struct {
	start, end int64
	dir        Direction
}, depth int64, code ErrorCode) *Error {
	if len(segments) == 0 {
		return NewError(code, "no segments supplied")
	}
	sorted := append([]struct {
		start, end int64
		dir        Direction
	}{}, segments...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].start < sorted[j].start })

	var reasons []Reason
	cursor := int64(0)
	for i, s := range sorted {
		if s.end <= s.start {
			reasons = append(reasons, Reason{
				DepthStart: s.start, DepthEnd: s.end, Code: code,
				Detail: "segment must have positive extent",
			})
			continue
		}
		if s.start != cursor {
			reasons = append(reasons, Reason{
				DepthStart: cursor, DepthEnd: s.start, Code: code,
				Detail: "segments have a gap or overlap",
			})
		}
		if i > 0 && s.dir != sorted[i-1].dir {
			reasons = append(reasons, Reason{
				DepthStart: s.start, DepthEnd: s.end, Code: code,
				Detail: "segment direction is inconsistent",
			})
		}
		cursor = s.end
	}
	if len(reasons) == 0 && cursor != depth {
		reasons = append(reasons, Reason{
			DepthStart: cursor, DepthEnd: depth, Code: code,
			Detail: "segments do not cover the full depth",
		})
	}
	if len(reasons) == 0 {
		return nil
	}
	return (&Error{Code: code, Message: "invalid segment coverage", Reasons: reasons}).Normalize()
}

// ValidateRebar checks the reinforcing-cage segments for continuous, non-
// overlapping, non-duplicated coverage of [0, designDepth] with a consistent
// direction.
func ValidateRebar(rebar []RebarSegment, designDepth int64) *Error {
	segments := make([]struct {
		start, end int64
		dir        Direction
	}, len(rebar))
	for i, r := range rebar {
		segments[i] = struct {
			start, end int64
			dir        Direction
		}{r.Start, r.End, r.Direction}
	}
	return validateSegments(segments, designDepth, CodeRebarLayoutInvalid)
}

// ValidateSonic checks that every acoustic tube covers a positive contiguous
// depth range, that its neighbour references point at existing tubes, and that
// the tube set is non-empty.
func ValidateSonic(sonic []SonicTube) *Error {
	if len(sonic) == 0 {
		return NewError(CodeRebarLayoutInvalid, "at least one acoustic tube is required")
	}
	ids := make(map[string]bool, len(sonic))
	for _, s := range sonic {
		ids[s.ID] = true
	}
	var reasons []Reason
	for _, s := range sonic {
		if s.End <= s.Start {
			reasons = append(reasons, Reason{
				DepthStart: s.Start, DepthEnd: s.End, Line: s.ID, Code: CodeRebarLayoutInvalid,
				Detail: "acoustic tube must have positive extent",
			})
		}
		for _, n := range s.Neighbors {
			if !ids[n] {
				reasons = append(reasons, Reason{
					Line: s.ID, Code: CodeRebarLayoutInvalid,
					Detail: "acoustic tube neighbour does not exist: " + n,
				})
			}
		}
	}
	if len(reasons) == 0 {
		return nil
	}
	return (&Error{Code: CodeRebarLayoutInvalid, Message: "invalid acoustic tube layout", Reasons: reasons}).Normalize()
}

// ValidateConduit checks the ordered conduit segments: positive lengths,
// non-duplicate indices, all water-tight, and a total active length that fits
// inside the measured hole depth.
func ValidateConduit(segments []ConduitSegment, holeDepth int64) *Error {
	if len(segments) == 0 {
		return NewError(CodeConduitDuplicate, "no conduit segments supplied")
	}
	var reasons []Reason
	seen := make(map[int]bool, len(segments))
	var total int64
	for _, s := range segments {
		if s.Length <= 0 {
			reasons = append(reasons, Reason{
				Segment: s.Index, Code: CodeConduitDuplicate, Detail: "segment length must be positive",
			})
		}
		if seen[s.Index] {
			reasons = append(reasons, Reason{
				Segment: s.Index, Code: CodeConduitDuplicate, Detail: "duplicate conduit segment index",
			})
		}
		seen[s.Index] = true
		if !s.Watertight {
			reasons = append(reasons, Reason{
				Segment: s.Index, Code: CodeConduitDuplicate, Detail: "segment is not water-tight",
			})
		}
		total += s.Length
	}
	if total > holeDepth {
		reasons = append(reasons, Reason{
			DepthStart: holeDepth, DepthEnd: total, Code: CodeConduitDuplicate,
			Detail: "conduit total length exceeds the measured hole depth",
		})
	}
	if len(reasons) == 0 {
		return nil
	}
	return (&Error{Code: CodeConduitDuplicate, Message: "invalid conduit assembly", Reasons: reasons}).Normalize()
}

// LayerAt returns the name of the stratum containing the given depth, or an
// empty string when no layer covers it.
func LayerAt(layers []Layer, depth int64) string {
	for _, l := range layers {
		if depth >= l.Start && depth < l.End {
			return l.Name
		}
	}
	return ""
}

// ValidateSnapshot performs the full locked-design validation on an already
// assembled snapshot. It is used both before locking and when re-validating a
// persisted snapshot.
func ValidateSnapshot(d DesignSnapshot) *Error {
	if d.Pier == "" || d.PileNo == "" {
		return NewError(CodeDesignMismatch, "pier and pile number are required")
	}
	if d.DesignDepth <= 0 || d.Diameter <= 0 {
		return NewError(CodeDesignMismatch, "design depth and diameter must be positive")
	}
	if err := ValidateLayers(d.Layers, d.DesignDepth); err != nil {
		return err
	}
	if err := ValidateRebar(d.Rebar, d.DesignDepth); err != nil {
		return err
	}
	if err := ValidateSonic(d.Sonic); err != nil {
		return err
	}
	if err := validateAdjacency(d.LineAdjacency, d.Sonic); err != nil {
		return err
	}
	if d.Pour.FirstPourVolume <= 0 || d.Pour.MinEmbedment <= 0 || d.Pour.MaxEmbedment < d.Pour.MinEmbedment {
		return NewError(CodeDesignMismatch, "invalid pour window")
	}
	return nil
}
