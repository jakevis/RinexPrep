package qc

import "fmt"

// AssessOPUS evaluates OPUS readiness and computes the OPUS score.
// Called after metrics are computed; mutates r in place.
func AssessOPUS(r *Report) {
	r.Warnings = []string{}
	r.Failures = []string{}

	// --- Failures (OPUS will reject) ---
	durationMin := r.DurationHours * 60
	if durationMin < 15 {
		r.Failures = append(r.Failures, fmt.Sprintf("Session duration %.1f min < 15 min minimum", durationMin))
	}
	if r.GPSSatMean < 4 {
		r.Failures = append(r.Failures, fmt.Sprintf("Average GPS satellites %.1f < 4 minimum", r.GPSSatMean))
	}
	if r.L2CoveragePct < 10 {
		r.Failures = append(r.Failures, "L2 coverage < 10% (dual-frequency required by OPUS)")
	}
	if r.MaxGapSec > 600 {
		r.Failures = append(r.Failures, fmt.Sprintf("Max gap %.0f s > 600 s", r.MaxGapSec))
	}

	// --- Warnings (may affect quality) ---
	if r.DurationHours < 2 {
		r.Warnings = append(r.Warnings, "Session shorter than 2 hours")
	} else if r.DurationHours < 4 {
		r.Warnings = append(r.Warnings, "Session shorter than 4 hours (recommended for best results)")
	}
	if r.GPSSatMean >= 4 && r.GPSSatMean < 6 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("Average GPS satellites %.1f is marginal (≥6 recommended)", r.GPSSatMean))
	}
	if r.L2CoveragePct >= 10 && r.L2CoveragePct < 50 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("L2 coverage %.0f%% is low (>80%% recommended)", r.L2CoveragePct))
	} else if r.L2CoveragePct >= 50 && r.L2CoveragePct < 80 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("L2 coverage %.0f%% is acceptable but >80%% recommended", r.L2CoveragePct))
	}
	if r.MaxGapSec > 120 && r.MaxGapSec <= 600 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("Max gap %.0f s > 120 s", r.MaxGapSec))
	}
	if r.ObsCompleteness > 0 && r.ObsCompleteness < 80 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("Observation completeness %.0f%% < 80%%", r.ObsCompleteness))
	}
	if r.CycleSlipCount > 50 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("High cycle slip count: %d (>50)", r.CycleSlipCount))
	}
	if r.DualFreqCount == 0 {
		r.Warnings = append(r.Warnings, "No dual-frequency satellites detected")
	}

	// --- Score ---
	r.OPUSScore = computeOPUSScore(r.DurationHours, r.GPSSatMean, r.L2CoveragePct)
	r.OPUSReady = len(r.Failures) == 0
}

// computeOPUSScore returns a 0–100 score based on three weighted components.
func computeOPUSScore(durHours, gpsMean, l2Pct float64) float64 {
	score := 0.0

	// Duration: 40% weight, full marks at 4+ hours, proportional below
	if durHours >= 4 {
		score += 40
	} else {
		score += 40 * (durHours / 4)
	}

	// GPS satellites: 30% weight, full marks at 8+ mean, proportional below
	if gpsMean >= 8 {
		score += 30
	} else {
		score += 30 * (gpsMean / 8)
	}

	// L2 coverage: 30% weight, full marks at 90%+, proportional below
	if l2Pct >= 90 {
		score += 30
	} else {
		score += 30 * (l2Pct / 90)
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
