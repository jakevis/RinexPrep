package pipeline

import (
	"strings"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// FilterConfig controls constellation filtering and minimum satellite thresholds.
type FilterConfig struct {
	Constellation string // "gps" (default), "all"
	MinSatellites int    // minimum sats per epoch (default 4), drop epoch if fewer
}

// DefaultFilterConfig returns sensible defaults for GPS-only processing.
func DefaultFilterConfig() FilterConfig {
	return FilterConfig{
		Constellation: "gps",
		MinSatellites: 4,
	}
}

// FilterConstellations filters epochs to the specified constellation.
// Also drops epochs with fewer than MinSatellites after filtering.
func FilterConstellations(epochs []gnss.Epoch, cfg FilterConfig) []gnss.Epoch {
	if len(epochs) == 0 {
		return nil
	}

	result := make([]gnss.Epoch, 0, len(epochs))
	for _, ep := range epochs {
		filtered := applyConstellationFilter(ep, cfg.Constellation)
		if len(filtered.Satellites) >= cfg.MinSatellites {
			result = append(result, filtered)
		}
	}
	return result
}

// applyConstellationFilter returns a filtered epoch based on constellation setting.
func applyConstellationFilter(ep gnss.Epoch, constellation string) gnss.Epoch {
	switch strings.ToLower(constellation) {
	case "gps":
		return ep.FilterGPSOnly()
	default: // "all" or unrecognized → keep everything
		return ep
	}
}
