package pipeline

import (
	"math"

	"github.com/jakevis/rinexprep/internal/gnss"
)

const (
	// Geometry-free combination thresholds
	gfSlipThresholdM = 0.15 // 15cm jump in GF → cycle slip on L1 or L2
	mwSlipThresholdCyc = 2.0 // 2-cycle jump in MW widelane → cycle slip

	// Carrier wavelengths
	lambda1 = cLight / freqL1 // ~0.1903m
	lambda2 = cLight / freqL2 // ~0.2442m
)

// SlipDetectConfig controls advanced cycle slip detection.
type SlipDetectConfig struct {
	// EnableGF enables geometry-free (ionosphere) combination slip detection.
	EnableGF bool
	// GFThresholdM is the geometry-free jump threshold in meters.
	GFThresholdM float64
	// EnableMW enables Melbourne-Wübbena widelane slip detection.
	EnableMW bool
	// MWThresholdCyc is the MW widelane jump threshold in cycles.
	MWThresholdCyc float64
}

// DefaultSlipDetectConfig returns sensible defaults.
func DefaultSlipDetectConfig() SlipDetectConfig {
	return SlipDetectConfig{
		EnableGF:       true,
		GFThresholdM:   gfSlipThresholdM,
		EnableMW:       true,
		MWThresholdCyc: mwSlipThresholdCyc,
	}
}

// slipState tracks per-satellite state for advanced slip detection.
type slipState struct {
	prevGF    float64 // previous geometry-free value (meters)
	prevMW    float64 // previous Melbourne-Wübbena value (cycles)
	prevValid bool    // whether previous values are valid
}

// DetectAdvancedSlips analyzes dual-frequency carrier phase for cycle slips
// using geometry-free and Melbourne-Wübbena combinations. Detected slips
// are marked by setting LockTimeSec=0 on the affected signal, which the
// downstream LLI logic will pick up.
//
// Geometry-free: GF = L1*λ1 - L2*λ2 (meters)
// The ionosphere is smooth — jumps > threshold indicate cycle slips.
//
// Melbourne-Wübbena: MW = (L1-L2) - (f1*P1+f2*P2)/(f1+f2) * (f1-f2)/c
// The MW widelane is nearly constant — jumps indicate slips.
func DetectAdvancedSlips(epochs []gnss.Epoch, cfg SlipDetectConfig) []gnss.Epoch {
	if !cfg.EnableGF && !cfg.EnableMW {
		return epochs
	}

	states := make(map[string]*slipState) // keyed by "G01"

	result := make([]gnss.Epoch, len(epochs))
	for i, ep := range epochs {
		result[i] = ep
		// Deep copy satellites so we can modify signals
		result[i].Satellites = make([]gnss.SatObs, len(ep.Satellites))
		for j, sat := range ep.Satellites {
			result[i].Satellites[j] = gnss.SatObs{
				Constellation: sat.Constellation,
				PRN:           sat.PRN,
				Signals:       make([]gnss.Signal, len(sat.Signals)),
			}
			copy(result[i].Satellites[j].Signals, sat.Signals)

			if sat.Constellation != gnss.ConsGPS {
				continue
			}

			key := sat.SatID()
			l1 := bestSignalForBandRO(sat.Signals, 0)
			l2 := bestSignalForBandRO(sat.Signals, 1)

			if l1 == nil || l2 == nil || !l1.CPValid || !l2.CPValid ||
				l1.CarrierPhase == 0 || l2.CarrierPhase == 0 {
				// Can't compute dual-freq combinations
				if st, ok := states[key]; ok {
					st.prevValid = false
				}
				continue
			}

			st := states[key]
			if st == nil {
				st = &slipState{}
				states[key] = st
			}

			slipDetected := false

			// Geometry-free combination
			if cfg.EnableGF {
				gf := l1.CarrierPhase*lambda1 - l2.CarrierPhase*lambda2
				if st.prevValid {
					dgf := math.Abs(gf - st.prevGF)
					if dgf > cfg.GFThresholdM {
						slipDetected = true
					}
				}
				st.prevGF = gf
			}

			// Melbourne-Wübbena widelane
			if cfg.EnableMW && l1.PRValid && l2.PRValid &&
				l1.Pseudorange != 0 && l2.Pseudorange != 0 {
				// MW = (L1 - L2) - (f1*P1 + f2*P2) / (f1 + f2) / λw
				// where λw = c / (f1 - f2)
				lambdaW := cLight / (freqL1 - freqL2)
				nw := (l1.CarrierPhase - l2.CarrierPhase) -
					(freqL1*l1.Pseudorange+freqL2*l2.Pseudorange)/(freqL1+freqL2)/lambdaW
				if st.prevValid {
					dmw := math.Abs(nw - st.prevMW)
					if dmw > cfg.MWThresholdCyc {
						slipDetected = true
					}
				}
				st.prevMW = nw
			}

			st.prevValid = true

			if slipDetected {
				// Mark both L1 and L2 signals with lock time reset
				// The writer's LLI logic will pick this up
				for k := range result[i].Satellites[j].Signals {
					sig := &result[i].Satellites[j].Signals[k]
					if sig.CPValid && sig.CarrierPhase != 0 {
						sig.LockTimeSec = 0
					}
				}
			}
		}
	}

	return result
}

// bestSignalForBandRO returns the best signal for a frequency band (read-only).
func bestSignalForBandRO(signals []gnss.Signal, band uint8) *gnss.Signal {
	var best *gnss.Signal
	for i := range signals {
		s := &signals[i]
		if s.FreqBand != band {
			continue
		}
		if best == nil || s.SigID < best.SigID {
			best = s
		}
	}
	return best
}
