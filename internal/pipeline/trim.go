package pipeline

import "github.com/jakevis/rinexprep/internal/gnss"

// TrimConfig specifies the time window for epoch trimming.
type TrimConfig struct {
	Start *gnss.GNSSTime // trim epochs before this (nil = no start trim)
	End   *gnss.GNSSTime // trim epochs after this (nil = no end trim)
}

// Trim removes epochs outside the specified time window.
func Trim(epochs []gnss.Epoch, cfg TrimConfig) []gnss.Epoch {
	if len(epochs) == 0 {
		return nil
	}

	result := make([]gnss.Epoch, 0, len(epochs))
	for _, ep := range epochs {
		t := ep.Time.UnixNanos()
		if cfg.Start != nil && t < cfg.Start.UnixNanos() {
			continue
		}
		if cfg.End != nil && t > cfg.End.UnixNanos() {
			continue
		}
		result = append(result, ep)
	}
	return result
}
