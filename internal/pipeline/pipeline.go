package pipeline

import "github.com/jakevis/rinexprep/internal/gnss"

// Config holds settings for the full normalization pipeline.
type Config struct {
	Normalize NormalizeConfig
	Filter    FilterConfig
	Trim      TrimConfig
}

// DefaultConfig returns sensible defaults for OPUS processing.
func DefaultConfig() Config {
	return Config{
		Normalize: DefaultNormalizeConfig(),
		Filter:    DefaultFilterConfig(),
		Trim:      TrimConfig{}, // no trimming by default
	}
}

// Stats records how many epochs survived each pipeline stage.
type Stats struct {
	InputEpochs      int
	AfterTrim        int
	AfterFilter      int
	AfterNormalize   int
	DroppedOffGrid   int
	DroppedLowSats   int
	DroppedDuplicate int
}

// Process runs the full pipeline: trim → filter → normalize.
// Returns processed epochs and processing stats.
func Process(epochs []gnss.Epoch, cfg Config) ([]gnss.Epoch, *Stats) {
	stats := &Stats{InputEpochs: len(epochs)}

	// Stage 1: trim
	trimmed := Trim(epochs, cfg.Trim)
	stats.AfterTrim = len(trimmed)

	// Stage 2: filter constellations
	filtered := FilterConstellations(trimmed, cfg.Filter)
	stats.AfterFilter = len(filtered)
	stats.DroppedLowSats = stats.AfterTrim - stats.AfterFilter

	// Stage 3: normalize (snap + dedup)
	normalized := Normalize(filtered, cfg.Normalize)
	stats.AfterNormalize = len(normalized)

	// Compute detailed drop reasons from normalize stage.
	// Count how many were off-grid vs duplicates.
	droppedTotal := stats.AfterFilter - stats.AfterNormalize
	offGrid := countOffGrid(filtered, cfg.Normalize.SnapToleranceNs)
	stats.DroppedOffGrid = offGrid
	stats.DroppedDuplicate = droppedTotal - offGrid

	return normalized, stats
}

// countOffGrid counts epochs whose grid offset exceeds the snap tolerance.
func countOffGrid(epochs []gnss.Epoch, toleranceNs int64) int {
	count := 0
	for _, ep := range epochs {
		offset := ep.Time.GridOffset30s()
		if offset < 0 {
			offset = -offset
		}
		if offset > toleranceNs {
			count++
		}
	}
	return count
}
