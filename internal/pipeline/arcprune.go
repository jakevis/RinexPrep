package pipeline

import "github.com/jakevis/rinexprep/internal/gnss"

// ArcPruneConfig controls boundary arc pruning to minimize short
// ambiguity segments at session edges.
type ArcPruneConfig struct {
	// MinArcDurationSec: minimum contiguous arc duration to keep.
	// Arcs shorter than this that touch the session boundary are pruned.
	// Default: 300s (5 minutes, matches typical OPUS minimum for fixing).
	MinArcDurationSec float64

	// BoundaryScanSec: how far from each boundary to scan for short arcs.
	// Default: 600s (10 minutes).
	BoundaryScanSec float64

	// MaxPruneSec: maximum time to trim from each boundary.
	// Prevents over-trimming. Default: 300s (5 minutes).
	MaxPruneSec float64

	// RequireDualFreq: only prune based on satellites that have L2.
	// L1-only sats can't contribute to ambiguity resolution anyway.
	RequireDualFreq bool
}

// DefaultArcPruneConfig returns sensible defaults for OPUS processing.
func DefaultArcPruneConfig() ArcPruneConfig {
	return ArcPruneConfig{
		MinArcDurationSec: 300,
		BoundaryScanSec:   600,
		MaxPruneSec:       300,
		RequireDualFreq:   true,
	}
}

// PruneShortBoundaryArcs trims session boundaries to remove satellites
// with short contiguous arcs that touch the edge, which create
// unresolvable ambiguity segments in OPUS.
//
// Operates on normalized 30s epochs where epoch count maps to time.
func PruneShortBoundaryArcs(epochs []gnss.Epoch, cfg ArcPruneConfig) []gnss.Epoch {
	if len(epochs) < 2 || cfg.MinArcDurationSec <= 0 {
		return epochs
	}

	sessionStart := epochs[0].Time.UnixNanos()
	sessionEnd := epochs[len(epochs)-1].Time.UnixNanos()
	sessionDurSec := float64(sessionEnd-sessionStart) / 1e9

	// Don't prune if session is too short (< 30 min)
	if sessionDurSec < 1800 {
		return epochs
	}

	scanNanos := int64(cfg.BoundaryScanSec * 1e9)
	maxPruneNanos := int64(cfg.MaxPruneSec * 1e9)

	// Find the latest short-arc end at the start boundary
	newStartNanos := sessionStart
	startCutoff := sessionStart + scanNanos
	arcs := findBoundaryArcs(epochs, sessionStart, startCutoff, true, cfg)
	for _, arc := range arcs {
		if arc.durationSec < cfg.MinArcDurationSec && arc.endNanos > newStartNanos {
			candidate := arc.endNanos + 1 // trim past this arc
			if candidate-sessionStart <= maxPruneNanos {
				newStartNanos = candidate
			}
		}
	}

	// Find the earliest short-arc start at the end boundary
	newEndNanos := sessionEnd
	endCutoff := sessionEnd - scanNanos
	arcs = findBoundaryArcs(epochs, endCutoff, sessionEnd, false, cfg)
	for _, arc := range arcs {
		if arc.durationSec < cfg.MinArcDurationSec && arc.startNanos < newEndNanos {
			candidate := arc.startNanos - 1
			if sessionEnd-candidate <= maxPruneNanos {
				newEndNanos = candidate
			}
		}
	}

	// Snap inward to 30s grid
	newStartNanos = snapNanosToGridCeil(newStartNanos, 30)
	newEndNanos = snapNanosToGridFloor(newEndNanos, 30)

	if newStartNanos >= newEndNanos {
		return epochs // pruning would collapse the session
	}

	// Filter epochs
	var result []gnss.Epoch
	for _, ep := range epochs {
		t := ep.Time.UnixNanos()
		if t >= newStartNanos && t <= newEndNanos {
			result = append(result, ep)
		}
	}

	return result
}

// boundaryArc represents a contiguous tracking arc for one satellite
// that touches a session boundary.
type boundaryArc struct {
	sat         string
	startNanos  int64
	endNanos    int64
	durationSec float64
}

// findBoundaryArcs finds contiguous arcs per satellite within the scan zone
// that touch the session boundary.
// touchStart=true means we look for arcs that START at or near the boundary.
// touchStart=false means arcs that END at or near the boundary.
func findBoundaryArcs(epochs []gnss.Epoch, zoneStart, zoneEnd int64, touchStart bool, cfg ArcPruneConfig) []boundaryArc {
	// Track per-satellite presence within the scan zone
	type satTrack struct {
		firstSeen int64
		lastSeen  int64
		hasDualFreq bool
	}
	tracks := make(map[string]*satTrack)

	for _, ep := range epochs {
		t := ep.Time.UnixNanos()
		if t < zoneStart || t > zoneEnd {
			continue
		}
		for _, sat := range ep.Satellites {
			if sat.Constellation != gnss.ConsGPS {
				continue
			}
			key := sat.SatID()
			hasL2 := false
			hasPhase := false
			for _, sig := range sat.Signals {
				if sig.CPValid && sig.CarrierPhase != 0 {
					hasPhase = true
					if sig.FreqBand == 1 {
						hasL2 = true
					}
				}
			}
			if !hasPhase {
				continue
			}
			if cfg.RequireDualFreq && !hasL2 {
				continue
			}

			if tracks[key] == nil {
				tracks[key] = &satTrack{firstSeen: t, lastSeen: t, hasDualFreq: hasL2}
			} else {
				if t < tracks[key].firstSeen {
					tracks[key].firstSeen = t
				}
				if t > tracks[key].lastSeen {
					tracks[key].lastSeen = t
				}
				if hasL2 {
					tracks[key].hasDualFreq = true
				}
			}
		}
	}

	var arcs []boundaryArc
	for sat, track := range tracks {
		dur := float64(track.lastSeen-track.firstSeen) / 1e9

		// Only include arcs that touch the boundary
		touchesBoundary := false
		if touchStart {
			// Arc must start within 60s of the boundary (satellite just rose)
			touchesBoundary = float64(track.firstSeen-zoneStart)/1e9 < 60
		} else {
			// Arc must end within 60s of the boundary (satellite setting)
			touchesBoundary = float64(zoneEnd-track.lastSeen)/1e9 < 60
		}

		if touchesBoundary {
			arcs = append(arcs, boundaryArc{
				sat:         sat,
				startNanos:  track.firstSeen,
				endNanos:    track.lastSeen,
				durationSec: dur,
			})
		}
	}
	return arcs
}

func snapNanosToGridCeil(nanos int64, intervalSec int) int64 {
	grid := int64(intervalSec) * 1e9
	mod := nanos % grid
	if mod == 0 {
		return nanos
	}
	return nanos + (grid - mod)
}

func snapNanosToGridFloor(nanos int64, intervalSec int) int64 {
	grid := int64(intervalSec) * 1e9
	mod := nanos % grid
	return nanos - mod
}
