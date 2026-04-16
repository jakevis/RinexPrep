package pipeline

import (
	"fmt"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// AutoTrimConfig controls the instability detection for survey start-up
// and teardown periods.
type AutoTrimConfig struct {
	// MinSatellites: epochs with fewer GPS sats than this are considered unstable.
	MinSatellites int
	// MinSNR: minimum average SNR (dB-Hz) for an epoch to be considered stable.
	MinSNR float64
	// StabilityWindow: number of consecutive stable epochs required before
	// we consider the survey "started" (and similarly at the end).
	StabilityWindow int
	// AlignToGrid: snap the trim boundaries to the nearest grid mark.
	AlignToGrid bool
	// GridIntervalSec: grid interval in seconds for alignment.
	GridIntervalSec int
}

// DefaultAutoTrimConfig returns an AutoTrimConfig with sensible defaults
// for typical GNSS survey equipment.
func DefaultAutoTrimConfig() AutoTrimConfig {
	return AutoTrimConfig{
		MinSatellites:   5,
		MinSNR:          30.0,
		StabilityWindow: 10,
		AlignToGrid:     true,
		GridIntervalSec: 30,
	}
}

// AutoTrimResult describes what AutoTrim detected and removed.
type AutoTrimResult struct {
	OriginalStart   gnss.GNSSTime
	OriginalEnd     gnss.GNSSTime
	TrimmedStart    gnss.GNSSTime
	TrimmedEnd      gnss.GNSSTime
	StartTrimmedSec float64
	EndTrimmedSec   float64
	EpochsRemoved   int
	Reason          string
}

// AutoTrim detects survey start-up and teardown instability and returns
// the trimmed epoch slice. Also returns the detected stable time window.
func AutoTrim(epochs []gnss.Epoch, cfg AutoTrimConfig) ([]gnss.Epoch, AutoTrimResult) {
	if len(epochs) == 0 {
		return nil, AutoTrimResult{Reason: "no epochs provided"}
	}

	originalStart := epochs[0].Time
	originalEnd := epochs[len(epochs)-1].Time

	// Find stable start: walk forward looking for StabilityWindow
	// consecutive stable epochs.
	stableStartIdx := findStableStart(epochs, cfg)

	// Find stable end: walk backward.
	stableEndIdx := findStableEnd(epochs, cfg)

	// All epochs unstable or stable window doesn't fit.
	if stableStartIdx < 0 || stableEndIdx < 0 || stableStartIdx > stableEndIdx {
		return nil, AutoTrimResult{
			OriginalStart: originalStart,
			OriginalEnd:   originalEnd,
			TrimmedStart:  originalStart,
			TrimmedEnd:    originalEnd,
			EpochsRemoved: len(epochs),
			Reason:        "all epochs unstable; no stable window found",
		}
	}

	trimmedStart := epochs[stableStartIdx].Time
	trimmedEnd := epochs[stableEndIdx].Time

	if cfg.AlignToGrid {
		trimmedStart = snapToGridCeil(trimmedStart, cfg.GridIntervalSec)
		trimmedEnd = snapToGridFloor(trimmedEnd, cfg.GridIntervalSec)
	}

	// After grid alignment the window may have collapsed.
	if trimmedStart.UnixNanos() > trimmedEnd.UnixNanos() {
		return nil, AutoTrimResult{
			OriginalStart: originalStart,
			OriginalEnd:   originalEnd,
			TrimmedStart:  trimmedStart,
			TrimmedEnd:    trimmedEnd,
			EpochsRemoved: len(epochs),
			Reason:        "stable window too short after grid alignment",
		}
	}

	// Collect epochs within [trimmedStart, trimmedEnd].
	var trimmed []gnss.Epoch
	startNanos := trimmedStart.UnixNanos()
	endNanos := trimmedEnd.UnixNanos()
	for _, e := range epochs {
		t := e.Time.UnixNanos()
		if t >= startNanos && t <= endNanos {
			trimmed = append(trimmed, e)
		}
	}

	startTrimmedSec := float64(trimmedStart.UnixNanos()-originalStart.UnixNanos()) / 1e9
	endTrimmedSec := float64(originalEnd.UnixNanos()-trimmedEnd.UnixNanos()) / 1e9

	reason := buildReason(stableStartIdx, len(epochs)-1-stableEndIdx, cfg)

	return trimmed, AutoTrimResult{
		OriginalStart:   originalStart,
		OriginalEnd:     originalEnd,
		TrimmedStart:    trimmedStart,
		TrimmedEnd:      trimmedEnd,
		StartTrimmedSec: startTrimmedSec,
		EndTrimmedSec:   endTrimmedSec,
		EpochsRemoved:   len(epochs) - len(trimmed),
		Reason:          reason,
	}
}

// findStableStart returns the index of the first epoch in the first run of
// StabilityWindow consecutive stable epochs. Returns -1 if none found.
func findStableStart(epochs []gnss.Epoch, cfg AutoTrimConfig) int {
	consecutive := 0
	for i := 0; i < len(epochs); i++ {
		if isStableEpoch(epochs[i], cfg.MinSatellites, cfg.MinSNR) {
			consecutive++
			if consecutive >= cfg.StabilityWindow {
				return i - cfg.StabilityWindow + 1
			}
		} else {
			consecutive = 0
		}
	}
	return -1
}

// findStableEnd returns the index of the last epoch in the last run of
// StabilityWindow consecutive stable epochs (walking backward). Returns -1
// if none found.
func findStableEnd(epochs []gnss.Epoch, cfg AutoTrimConfig) int {
	consecutive := 0
	for i := len(epochs) - 1; i >= 0; i-- {
		if isStableEpoch(epochs[i], cfg.MinSatellites, cfg.MinSNR) {
			consecutive++
			if consecutive >= cfg.StabilityWindow {
				return i + cfg.StabilityWindow - 1
			}
		} else {
			consecutive = 0
		}
	}
	return -1
}

// isStableEpoch checks if an epoch meets stability criteria.
func isStableEpoch(epoch gnss.Epoch, minSats int, minSNR float64) bool {
	if epoch.GPSSatCount() < minSats {
		return false
	}
	return averageGPSSNR(epoch) >= minSNR
}

// averageGPSSNR computes the mean SNR across all GPS signals in an epoch.
func averageGPSSNR(epoch gnss.Epoch) float64 {
	var total float64
	var count int
	for _, sat := range epoch.Satellites {
		if sat.Constellation == gnss.ConsGPS {
			for _, sig := range sat.Signals {
				total += sig.SNR
				count++
			}
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// snapToGridCeil returns a GNSSTime snapped up to the next grid boundary
// at or after t, so we don't include partial grid intervals at the start.
func snapToGridCeil(t gnss.GNSSTime, intervalSec int) gnss.GNSSTime {
	gridNanos := int64(intervalSec) * 1_000_000_000
	mod := t.TOWNanos % gridNanos
	if mod < 0 {
		mod += gridNanos
	}
	if mod == 0 {
		return t // already on grid
	}
	result := t
	result.TOWNanos = t.TOWNanos + (gridNanos - mod)

	const nanosPerWeek = int64(7*24*3600) * 1_000_000_000
	if result.TOWNanos >= nanosPerWeek {
		result.TOWNanos -= nanosPerWeek
		result.Week++
	}
	return result
}

// snapToGridFloor returns a GNSSTime snapped down to the previous grid
// boundary at or before t, so we don't include partial grid intervals at the end.
func snapToGridFloor(t gnss.GNSSTime, intervalSec int) gnss.GNSSTime {
	gridNanos := int64(intervalSec) * 1_000_000_000
	mod := t.TOWNanos % gridNanos
	if mod < 0 {
		mod += gridNanos
	}
	result := t
	result.TOWNanos = t.TOWNanos - mod

	const nanosPerWeek = int64(7*24*3600) * 1_000_000_000
	if result.TOWNanos < 0 {
		result.TOWNanos += nanosPerWeek
		result.Week--
	}
	return result
}

func buildReason(startRemoved, endRemoved int, cfg AutoTrimConfig) string {
	var reason string
	switch {
	case startRemoved > 0 && endRemoved > 0:
		reason = fmt.Sprintf("trimmed %d unstable epochs from start and %d from end", startRemoved, endRemoved)
	case startRemoved > 0:
		reason = fmt.Sprintf("trimmed %d unstable epochs from start", startRemoved)
	case endRemoved > 0:
		reason = fmt.Sprintf("trimmed %d unstable epochs from end", endRemoved)
	default:
		reason = "no trimming needed"
	}
	if cfg.AlignToGrid {
		reason += fmt.Sprintf("; aligned to %ds grid", cfg.GridIntervalSec)
	}
	return reason
}
