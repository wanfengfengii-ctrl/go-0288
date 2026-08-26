package domain

import "sort"

// ExpandToLayers widens a depth range to the boundaries of the strata it
// intersects, per domain rule 8.
func ExpandToLayers(layers []Layer, r DepthRange) DepthRange {
	for _, l := range layers {
		if r.Start >= l.Start && r.Start <= l.End {
			r.Start = l.Start
			break
		}
	}
	for _, l := range layers {
		if r.End >= l.Start && r.End <= l.End {
			r.End = l.End
			break
		}
	}
	return r
}

// MergeRanges sorts and merges overlapping or adjacent depth ranges so the
// re-inspection set is minimal and stable.
func MergeRanges(rs []DepthRange) []DepthRange {
	if len(rs) == 0 {
		return nil
	}
	sorted := append([]DepthRange(nil), rs...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return sorted[i].End < sorted[j].End
	})
	out := []DepthRange{sorted[0]}
	for _, r := range sorted[1:] {
		last := &out[len(out)-1]
		if r.Start <= last.End {
			if r.End > last.End {
				last.End = r.End
			}
		} else {
			out = append(out, r)
		}
	}
	return out
}

// AdjacentLines returns the acoustic lines neighbouring the given line, plus
// the line itself, in deterministic order.
func AdjacentLines(adj [][]string, line string) []string {
	set := map[string]bool{line: true}
	for _, pair := range adj {
		if len(pair) == 2 && (pair[0] == line || pair[1] == line) {
			set[pair[0]] = true
			set[pair[1]] = true
		}
	}
	out := make([]string, 0, len(set))
	for l := range set {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// ExpandAnomalies deterministically derives the merged re-inspection depth set
// and the involved line set from acoustic integrity results: anomalous ranges
// are widened to stratum boundaries and merged, and acoustic anomalies include
// their neighbouring lines.
func ExpandAnomalies(d DesignSnapshot, lines []LineResult) ([]DepthRange, []string) {
	var raw []DepthRange
	lineSet := map[string]bool{}
	for _, l := range lines {
		if !l.Anomalous {
			continue
		}
		for _, r := range l.AnomalyRanges {
			raw = append(raw, ExpandToLayers(d.Layers, r))
		}
		for _, n := range AdjacentLines(d.LineAdjacency, l.Line) {
			lineSet[n] = true
		}
	}
	merged := MergeRanges(raw)
	linesOut := make([]string, 0, len(lineSet))
	for l := range lineSet {
		linesOut = append(linesOut, l)
	}
	sort.Strings(linesOut)
	return merged, linesOut
}
