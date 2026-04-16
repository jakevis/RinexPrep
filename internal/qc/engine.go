package qc

import (
	"math"
	"sort"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// Report contains all QC metrics for a GNSS observation session.
type Report struct {
	// Session info
	TotalEpochs   int
	DurationHours float64
	IntervalSec   float64 // estimated observation interval

	// GPS satellite metrics
	GPSSatMin  int
	GPSSatMax  int
	GPSSatMean float64

	// Signal metrics
	L1SatCount    int     // unique sats with L1
	L2SatCount    int     // unique sats with L2
	L5SatCount    int     // unique sats with L5
	DualFreqCount int     // sats with both L1+L2
	L2CoveragePct float64 // % of epochs with L2

	// SNR metrics
	MeanSNR   float64
	MeanSNRL1 float64
	MeanSNRL2 float64

	// Data quality
	MaxGapSec       float64 // largest gap between consecutive epochs
	GapCount        int     // number of gaps > 2x interval
	ObsCompleteness float64 // actual epochs / expected epochs (%)

	// Cycle slip estimates
	CycleSlipCount int // detected from lock time resets

	// OPUS readiness
	OPUSReady bool
	OPUSScore float64 // 0-100
	Warnings  []string
	Failures  []string
}

// satKey uniquely identifies a satellite by constellation and PRN.
type satKey struct {
	cons gnss.Constellation
	prn  uint8
}

// Analyze runs the full QC analysis on a set of epochs.
func Analyze(epochs []gnss.Epoch) *Report {
	r := &Report{
		Warnings: []string{},
		Failures: []string{},
	}

	if len(epochs) == 0 {
		return r
	}

	r.TotalEpochs = len(epochs)

	// 1. Duration
	startNanos := epochs[0].Time.UnixNanos()
	endNanos := epochs[len(epochs)-1].Time.UnixNanos()
	durationNanos := endNanos - startNanos
	r.DurationHours = float64(durationNanos) / 1e9 / 3600

	// 2. Interval — median of first min(100, n-1) epoch spacings
	r.IntervalSec = estimateInterval(epochs)

	// 3. GPS satellite stats (min/max/mean)
	var gpsSatSum int
	r.GPSSatMin = math.MaxInt32
	r.GPSSatMax = 0
	for _, ep := range epochs {
		c := ep.GPSSatCount()
		gpsSatSum += c
		if c < r.GPSSatMin {
			r.GPSSatMin = c
		}
		if c > r.GPSSatMax {
			r.GPSSatMax = c
		}
	}
	r.GPSSatMean = float64(gpsSatSum) / float64(r.TotalEpochs)
	if r.GPSSatMin == math.MaxInt32 {
		r.GPSSatMin = 0
	}

	// 4. Signal counts — unique (constellation, PRN) per frequency band
	l1Sats := make(map[satKey]bool)
	l2Sats := make(map[satKey]bool)
	l5Sats := make(map[satKey]bool)

	// 5. L2 coverage
	var epochsWithL2 int

	// 6. SNR accumulators
	var snrTotal, snrL1Total, snrL2Total float64
	var snrCount, snrL1Count, snrL2Count int

	// 10. Cycle slips — track per-satellite last lock time
	type sigKey struct {
		cons     gnss.Constellation
		prn      uint8
		freqBand uint8
	}
	lastLock := make(map[sigKey]float64)
	cycleSlips := 0

	for _, ep := range epochs {
		hasL2 := false
		for _, sat := range ep.Satellites {
			sk := satKey{sat.Constellation, sat.PRN}
			for _, sig := range sat.Signals {
				// Signal counts
				switch sig.FreqBand {
				case 0:
					l1Sats[sk] = true
				case 1:
					l2Sats[sk] = true
					if sat.Constellation == gnss.ConsGPS {
						hasL2 = true
					}
				case 2:
					l5Sats[sk] = true
				}

				// SNR
				if sig.SNR > 0 {
					snrTotal += sig.SNR
					snrCount++
					switch sig.FreqBand {
					case 0:
						snrL1Total += sig.SNR
						snrL1Count++
					case 1:
						snrL2Total += sig.SNR
						snrL2Count++
					}
				}

				// Cycle slip detection from lock time resets
				skey := sigKey{sat.Constellation, sat.PRN, sig.FreqBand}
				if prev, ok := lastLock[skey]; ok {
					if sig.LockTimeSec < prev {
						cycleSlips++
					}
				}
				lastLock[skey] = sig.LockTimeSec
			}
		}
		if hasL2 {
			epochsWithL2++
		}
	}

	r.L1SatCount = len(l1Sats)
	r.L2SatCount = len(l2Sats)
	r.L5SatCount = len(l5Sats)

	// Dual-frequency: sats with both L1 and L2
	for sk := range l1Sats {
		if l2Sats[sk] {
			r.DualFreqCount++
		}
	}

	r.L2CoveragePct = float64(epochsWithL2) / float64(r.TotalEpochs) * 100

	// SNR means
	if snrCount > 0 {
		r.MeanSNR = snrTotal / float64(snrCount)
	}
	if snrL1Count > 0 {
		r.MeanSNRL1 = snrL1Total / float64(snrL1Count)
	}
	if snrL2Count > 0 {
		r.MeanSNRL2 = snrL2Total / float64(snrL2Count)
	}

	// 7 & 8. Max gap and gap count
	for i := 1; i < len(epochs); i++ {
		gap := float64(epochs[i].Time.UnixNanos()-epochs[i-1].Time.UnixNanos()) / 1e9
		if gap > r.MaxGapSec {
			r.MaxGapSec = gap
		}
		if r.IntervalSec > 0 && gap > 2*r.IntervalSec {
			r.GapCount++
		}
	}

	// 9. Observation completeness
	if r.IntervalSec > 0 && r.DurationHours > 0 {
		expectedEpochs := (float64(durationNanos) / 1e9) / r.IntervalSec
		if expectedEpochs > 0 {
			r.ObsCompleteness = float64(r.TotalEpochs) / expectedEpochs * 100
			if r.ObsCompleteness > 100 {
				r.ObsCompleteness = 100
			}
		}
	}

	r.CycleSlipCount = cycleSlips

	// OPUS readiness assessment
	AssessOPUS(r)

	return r
}

// estimateInterval computes the median epoch spacing from the first N epoch pairs.
func estimateInterval(epochs []gnss.Epoch) float64 {
	if len(epochs) < 2 {
		return 0
	}

	n := len(epochs) - 1
	if n > 100 {
		n = 100
	}

	spacings := make([]float64, n)
	for i := 0; i < n; i++ {
		spacings[i] = float64(epochs[i+1].Time.UnixNanos()-epochs[i].Time.UnixNanos()) / 1e9
	}

	sort.Float64s(spacings)
	if n%2 == 0 {
		return (spacings[n/2-1] + spacings[n/2]) / 2
	}
	return spacings[n/2]
}
