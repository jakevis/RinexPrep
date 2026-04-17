package pipeline

import (
	"sort"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// NormalizeConfig controls grid snapping and deduplication.
type NormalizeConfig struct {
	IntervalSec     int   // grid interval (default 30)
	SnapToleranceNs int64 // max offset from grid to snap (default 100ms = 100_000_000)
}

// DefaultNormalizeConfig returns sensible defaults for 30-second grids.
func DefaultNormalizeConfig() NormalizeConfig {
	return NormalizeConfig{
		IntervalSec:     30,
		SnapToleranceNs: 5_000_000_000, // 5s — fills gaps from missing 1Hz epochs (dedup keeps closest)
	}
}

// gridKey uniquely identifies a grid point for deduplication.
type gridKey struct {
	Week     uint16
	TOWNanos int64
}

// candidate tracks an epoch with its original offset for dedup selection.
type candidate struct {
	epoch     gnss.Epoch
	absOffset int64
}

// Normalize takes raw epochs and returns grid-snapped, deduplicated epochs.
// Epochs outside snap tolerance are dropped.
// If multiple epochs snap to the same grid point, keep the closest.
func Normalize(epochs []gnss.Epoch, cfg NormalizeConfig) []gnss.Epoch {
	if len(epochs) == 0 {
		return nil
	}

	// 1. Sort by GNSSTime (Week, then TOWNanos).
	sorted := make([]gnss.Epoch, len(epochs))
	copy(sorted, epochs)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Time.Week != sorted[j].Time.Week {
			return sorted[i].Time.Week < sorted[j].Time.Week
		}
		return sorted[i].Time.TOWNanos < sorted[j].Time.TOWNanos
	})

	// 2-5. Snap to grid, drop out-of-tolerance, dedup by grid point.
	best := make(map[gridKey]candidate)
	for _, ep := range sorted {
		offset := ep.Time.GridOffset30s()
		absOff := offset
		if absOff < 0 {
			absOff = -absOff
		}

		// Drop if outside tolerance.
		if absOff > cfg.SnapToleranceNs {
			continue
		}

		// Snap the epoch's time.
		snapped := ep
		snapped.Time = ep.Time.SnapToGrid30s()
		key := gridKey{Week: snapped.Time.Week, TOWNanos: snapped.Time.TOWNanos}

		// Keep the epoch closest to the grid point.
		if prev, exists := best[key]; !exists || absOff < prev.absOffset {
			best[key] = candidate{epoch: snapped, absOffset: absOff}
		}
	}

	// 6-7. Collect, sort, and verify monotonic output.
	result := make([]gnss.Epoch, 0, len(best))
	for _, c := range best {
		result = append(result, c.epoch)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Time.Week != result[j].Time.Week {
			return result[i].Time.Week < result[j].Time.Week
		}
		return result[i].Time.TOWNanos < result[j].Time.TOWNanos
	})

	return result
}
