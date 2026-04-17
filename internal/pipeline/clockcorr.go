package pipeline

import (
	"math"

	"github.com/jakevis/rinexprep/internal/gnss"
)

const (
	cLight = 299792458.0 // speed of light (m/s)
	freqL1 = 1.57542e9   // GPS L1 frequency (Hz)
	freqL2 = 1.22760e9   // GPS L2 frequency (Hz)
)

// ClockCorrConfig controls receiver clock bias correction.
type ClockCorrConfig struct {
	// TADJ is the time adjustment interval in seconds (matches RTKLIB -TADJ).
	// The receiver time is rounded to the nearest multiple of TADJ.
	// Use 0.1 for Emlid/RTKLIB compatibility. Set to 0 to disable.
	TADJ float64
}

// signalFreq returns the carrier frequency in Hz for a GPS signal band.
func signalFreq(band uint8) float64 {
	switch band {
	case 0:
		return freqL1
	case 1:
		return freqL2
	default:
		return freqL1
	}
}

// CorrectClockBias applies RTKLIB-style time tag adjustment to epochs.
// This rounds epoch timestamps to the nearest TADJ grid point so that
// epochs align cleanly with the observation interval grid.
// Pseudorange and carrier phase values are NOT modified — they retain
// the receiver clock bias which OPUS estimates and removes during processing.
func CorrectClockBias(epochs []gnss.Epoch, cfg ClockCorrConfig) []gnss.Epoch {
	if cfg.TADJ <= 0 || len(epochs) == 0 {
		return epochs
	}

	result := make([]gnss.Epoch, len(epochs))
	for i, ep := range epochs {
		tow := float64(ep.Time.TOWNanos) / 1e9 // seconds
		tn := tow / cfg.TADJ
		toff := (tn - math.Floor(tn+0.5)) * cfg.TADJ

		corrected := ep
		corrected.Time.TOWNanos = int64(math.Round((tow - toff) * 1e9))

		// Deep copy satellites (no measurement modification)
		corrected.Satellites = make([]gnss.SatObs, len(ep.Satellites))
		copy(corrected.Satellites, ep.Satellites)

		result[i] = corrected
	}
	return result
}
