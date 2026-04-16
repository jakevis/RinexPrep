package qc

import (
	"math"
	"testing"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// makeEpochs builds a slice of epochs with configurable parameters.
// durationHours: session length in hours
// intervalSec: spacing between epochs
// gpsSats: number of GPS satellites per epoch
// includeL2: if true, every GPS sat gets an L2 signal
// lockTimes: if non-nil, overrides lock time on first signal of first sat each epoch
func makeEpochs(durationHours float64, intervalSec float64, gpsSats int, includeL2 bool, lockTimes []float64) []gnss.Epoch {
	durationNanos := int64(durationHours * 3600 * 1e9)
	intervalNanos := int64(intervalSec * 1e9)
	if intervalNanos <= 0 {
		intervalNanos = 30e9
	}

	n := int(durationNanos/intervalNanos) + 1
	if n < 1 {
		n = 1
	}

	epochs := make([]gnss.Epoch, n)
	for i := 0; i < n; i++ {
		towNanos := int64(i) * intervalNanos
		week := uint16(towNanos / (7 * 24 * 3600 * 1e9))
		towNanos = towNanos % (7 * 24 * 3600 * int64(1e9))

		ep := gnss.Epoch{
			Time: gnss.GNSSTime{
				Week:     week,
				TOWNanos: towNanos,
			},
		}

		for s := 1; s <= gpsSats; s++ {
			sat := gnss.SatObs{
				Constellation: gnss.ConsGPS,
				PRN:           uint8(s),
			}

			lockTime := float64(i) * intervalSec
			if lockTimes != nil && i < len(lockTimes) {
				lockTime = lockTimes[i]
			}

			// L1 signal
			sat.Signals = append(sat.Signals, gnss.Signal{
				GnssID:      0,
				SigID:       0,
				FreqBand:    0,
				SNR:         40.0,
				LockTimeSec: lockTime,
				PRValid:     true,
				CPValid:     true,
			})

			if includeL2 {
				sat.Signals = append(sat.Signals, gnss.Signal{
					GnssID:      0,
					SigID:       3,
					FreqBand:    1,
					SNR:         35.0,
					LockTimeSec: lockTime,
					PRValid:     true,
					CPValid:     true,
				})
			}

			ep.Satellites = append(ep.Satellites, sat)
		}
		epochs[i] = ep
	}
	return epochs
}

func TestPerfectData(t *testing.T) {
	epochs := makeEpochs(4.0, 30.0, 10, true, nil)
	r := Analyze(epochs)

	if !r.OPUSReady {
		t.Errorf("expected OPUSReady=true, got false; failures=%v", r.Failures)
	}
	if r.OPUSScore < 95 {
		t.Errorf("expected score >= 95, got %.1f", r.OPUSScore)
	}
	if r.GPSSatMean != 10 {
		t.Errorf("expected GPSSatMean=10, got %.1f", r.GPSSatMean)
	}
	if r.L2CoveragePct != 100 {
		t.Errorf("expected L2CoveragePct=100, got %.1f", r.L2CoveragePct)
	}
	if r.L1SatCount != 10 {
		t.Errorf("expected L1SatCount=10, got %d", r.L1SatCount)
	}
	if r.L2SatCount != 10 {
		t.Errorf("expected L2SatCount=10, got %d", r.L2SatCount)
	}
	if r.DualFreqCount != 10 {
		t.Errorf("expected DualFreqCount=10, got %d", r.DualFreqCount)
	}
	if len(r.Failures) > 0 {
		t.Errorf("expected no failures, got %v", r.Failures)
	}
}

func TestShortData(t *testing.T) {
	// 30 minutes — should warn but not fail (OPUS-RS capable)
	epochs := makeEpochs(0.5, 30.0, 10, true, nil)
	r := Analyze(epochs)

	if !r.OPUSReady {
		t.Errorf("expected OPUSReady=true for 30 min, got false; failures=%v", r.Failures)
	}
	if len(r.Warnings) == 0 {
		t.Error("expected warnings for short session")
	}
	found := false
	for _, w := range r.Warnings {
		if len(w) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected at least one non-empty warning")
	}
}

func TestVeryShortData(t *testing.T) {
	// 10 minutes — OPUS failure
	epochs := makeEpochs(10.0/60.0, 30.0, 10, true, nil)
	r := Analyze(epochs)

	if r.OPUSReady {
		t.Error("expected OPUSReady=false for 10 min session")
	}
	foundDurationFailure := false
	for _, f := range r.Failures {
		if len(f) > 0 {
			foundDurationFailure = true
		}
	}
	if !foundDurationFailure {
		t.Error("expected duration failure")
	}
}

func TestLowSats(t *testing.T) {
	// Mean 3 GPS sats — failure
	epochs := makeEpochs(4.0, 30.0, 3, true, nil)
	r := Analyze(epochs)

	if r.OPUSReady {
		t.Error("expected OPUSReady=false for mean 3 GPS sats")
	}
	if r.GPSSatMean != 3 {
		t.Errorf("expected GPSSatMean=3, got %.1f", r.GPSSatMean)
	}
}

func TestNoL2(t *testing.T) {
	epochs := makeEpochs(4.0, 30.0, 10, false, nil)
	r := Analyze(epochs)

	if r.OPUSReady {
		t.Error("expected OPUSReady=false for 0% L2 coverage")
	}
	if r.L2CoveragePct != 0 {
		t.Errorf("expected L2CoveragePct=0, got %.1f", r.L2CoveragePct)
	}
	if r.L2SatCount != 0 {
		t.Errorf("expected L2SatCount=0, got %d", r.L2SatCount)
	}
}

func TestLargeGap(t *testing.T) {
	// Build normal epochs then inject a 700s gap
	epochs := makeEpochs(4.0, 30.0, 10, true, nil)
	if len(epochs) > 10 {
		// Shift epoch 10 onward by 700 seconds
		shiftNanos := int64(700 * 1e9)
		for i := 10; i < len(epochs); i++ {
			epochs[i].Time.TOWNanos += shiftNanos
		}
	}

	r := Analyze(epochs)

	if r.OPUSReady {
		t.Error("expected OPUSReady=false for 700s gap")
	}
	if r.MaxGapSec < 700 {
		t.Errorf("expected MaxGapSec >= 700, got %.1f", r.MaxGapSec)
	}
}

func TestCycleSlipDetection(t *testing.T) {
	// Create epochs with lock time resets indicating cycle slips.
	// Lock times: 30, 60, 90, 10 (reset!), 40, 70, 5 (reset!), 35, 65
	lockTimes := []float64{30, 60, 90, 10, 40, 70, 5, 35, 65}
	epochs := make([]gnss.Epoch, len(lockTimes))
	for i, lt := range lockTimes {
		epochs[i] = gnss.Epoch{
			Time: gnss.GNSSTime{
				Week:     0,
				TOWNanos: int64(i) * 30e9,
			},
			Satellites: []gnss.SatObs{
				{
					Constellation: gnss.ConsGPS,
					PRN:           1,
					Signals: []gnss.Signal{
						{FreqBand: 0, SNR: 40, LockTimeSec: lt, PRValid: true, CPValid: true},
						{FreqBand: 1, SNR: 35, LockTimeSec: lt, PRValid: true, CPValid: true},
					},
				},
			},
		}
	}

	r := Analyze(epochs)

	// 2 lock time resets on L1 + 2 on L2 = 4 cycle slips
	if r.CycleSlipCount != 4 {
		t.Errorf("expected 4 cycle slips, got %d", r.CycleSlipCount)
	}
}

func TestEmptyEpochs(t *testing.T) {
	r := Analyze(nil)
	if r.TotalEpochs != 0 {
		t.Errorf("expected 0 epochs, got %d", r.TotalEpochs)
	}
	if r.OPUSReady {
		t.Error("expected OPUSReady=false for empty data")
	}
}

func TestIntervalEstimation(t *testing.T) {
	epochs := makeEpochs(1.0, 15.0, 5, true, nil)
	r := Analyze(epochs)
	if math.Abs(r.IntervalSec-15.0) > 0.01 {
		t.Errorf("expected IntervalSec=15, got %.2f", r.IntervalSec)
	}
}

func TestObsCompleteness(t *testing.T) {
	epochs := makeEpochs(1.0, 30.0, 8, true, nil)
	r := Analyze(epochs)
	// With uniform spacing, completeness should be ~100%
	if r.ObsCompleteness < 99 {
		t.Errorf("expected ~100%% completeness, got %.1f%%", r.ObsCompleteness)
	}
}

func TestGapCount(t *testing.T) {
	// Create epochs with two large gaps (> 2x interval)
	epochs := makeEpochs(1.0, 30.0, 8, true, nil)
	if len(epochs) > 20 {
		// Insert a 90s gap (> 2x30s) at epoch 10
		for i := 10; i < len(epochs); i++ {
			epochs[i].Time.TOWNanos += int64(90 * 1e9)
		}
		// Insert another 120s gap at epoch 20
		for i := 20; i < len(epochs); i++ {
			epochs[i].Time.TOWNanos += int64(120 * 1e9)
		}
	}
	r := Analyze(epochs)
	if r.GapCount < 2 {
		t.Errorf("expected at least 2 gaps > 2x interval, got %d", r.GapCount)
	}
}
