package pipeline

import (
	"testing"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// makeAutoTrimEpoch builds a test epoch at the given TOW second with the specified
// number of GPS satellites, all reporting the same SNR.
func makeAutoTrimEpoch(week uint16, towSec float64, gpsSats int, snr float64) gnss.Epoch {
	sats := make([]gnss.SatObs, gpsSats)
	for i := range sats {
		sats[i] = gnss.SatObs{
			Constellation: gnss.ConsGPS,
			PRN:           uint8(i + 1),
			Signals: []gnss.Signal{
				{GnssID: 0, SNR: snr, PRValid: true, CPValid: true},
			},
		}
	}
	return gnss.Epoch{
		Time: gnss.GNSSTime{
			Week:     week,
			TOWNanos: int64(towSec * 1e9),
		},
		Satellites: sats,
	}
}

// noGridConfig returns a config with grid alignment disabled for easier
// reasoning about trim boundaries.
func noGridConfig() AutoTrimConfig {
	cfg := DefaultAutoTrimConfig()
	cfg.AlignToGrid = false
	return cfg
}

func TestAutoTrim_AllStable(t *testing.T) {
	// 20 stable epochs, all meet criteria — nothing should be trimmed.
	epochs := make([]gnss.Epoch, 20)
	for i := range epochs {
		epochs[i] = makeAutoTrimEpoch(2300, float64(i), 8, 40.0)
	}
	cfg := noGridConfig()
	cfg.StabilityWindow = 10

	trimmed, result := AutoTrim(epochs, cfg)

	if len(trimmed) != len(epochs) {
		t.Errorf("expected %d epochs, got %d", len(epochs), len(trimmed))
	}
	if result.EpochsRemoved != 0 {
		t.Errorf("expected 0 removed, got %d", result.EpochsRemoved)
	}
	if result.StartTrimmedSec != 0 {
		t.Errorf("expected 0s start trim, got %f", result.StartTrimmedSec)
	}
	if result.EndTrimmedSec != 0 {
		t.Errorf("expected 0s end trim, got %f", result.EndTrimmedSec)
	}
}

func TestAutoTrim_UnstableStart_LowSats(t *testing.T) {
	// First 5 epochs have only 2 GPS sats (below default min of 5),
	// followed by 15 stable epochs.
	var epochs []gnss.Epoch
	for i := 0; i < 5; i++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), 2, 40.0))
	}
	for i := 5; i < 20; i++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), 8, 40.0))
	}
	cfg := noGridConfig()
	cfg.StabilityWindow = 10

	trimmed, result := AutoTrim(epochs, cfg)

	if result.EpochsRemoved != 5 {
		t.Errorf("expected 5 removed, got %d", result.EpochsRemoved)
	}
	if len(trimmed) != 15 {
		t.Errorf("expected 15 epochs, got %d", len(trimmed))
	}
	// First remaining epoch should be at second 5.
	if trimmed[0].Time.TOWSeconds() != 5.0 {
		t.Errorf("expected trimmed start at 5s, got %f", trimmed[0].Time.TOWSeconds())
	}
	if result.StartTrimmedSec != 5.0 {
		t.Errorf("expected 5s start trim, got %f", result.StartTrimmedSec)
	}
	if result.EndTrimmedSec != 0 {
		t.Errorf("expected 0s end trim, got %f", result.EndTrimmedSec)
	}
}

func TestAutoTrim_UnstableEnd_LowSNR(t *testing.T) {
	// 15 stable epochs, then 5 with low SNR (20 dB-Hz).
	var epochs []gnss.Epoch
	for i := 0; i < 15; i++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), 8, 40.0))
	}
	for i := 15; i < 20; i++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), 8, 20.0))
	}
	cfg := noGridConfig()
	cfg.StabilityWindow = 10

	trimmed, result := AutoTrim(epochs, cfg)

	if result.EpochsRemoved != 5 {
		t.Errorf("expected 5 removed, got %d", result.EpochsRemoved)
	}
	if len(trimmed) != 15 {
		t.Errorf("expected 15 epochs, got %d", len(trimmed))
	}
	if trimmed[len(trimmed)-1].Time.TOWSeconds() != 14.0 {
		t.Errorf("expected trimmed end at 14s, got %f", trimmed[len(trimmed)-1].Time.TOWSeconds())
	}
	if result.EndTrimmedSec != 5.0 {
		t.Errorf("expected 5s end trim, got %f", result.EndTrimmedSec)
	}
}

func TestAutoTrim_BothUnstable(t *testing.T) {
	// 5 unstable + 20 stable + 5 unstable.
	var epochs []gnss.Epoch
	for i := 0; i < 5; i++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), 2, 40.0))
	}
	for i := 5; i < 25; i++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), 8, 40.0))
	}
	for i := 25; i < 30; i++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), 8, 20.0))
	}
	cfg := noGridConfig()
	cfg.StabilityWindow = 10

	trimmed, result := AutoTrim(epochs, cfg)

	if result.EpochsRemoved != 10 {
		t.Errorf("expected 10 removed, got %d", result.EpochsRemoved)
	}
	if len(trimmed) != 20 {
		t.Errorf("expected 20 epochs, got %d", len(trimmed))
	}
	if result.StartTrimmedSec != 5.0 {
		t.Errorf("expected 5s start trim, got %f", result.StartTrimmedSec)
	}
	if result.EndTrimmedSec != 5.0 {
		t.Errorf("expected 5s end trim, got %f", result.EndTrimmedSec)
	}
}

func TestAutoTrim_AllUnstable(t *testing.T) {
	epochs := make([]gnss.Epoch, 15)
	for i := range epochs {
		epochs[i] = makeAutoTrimEpoch(2300, float64(i), 2, 20.0) // low sats AND low SNR
	}
	cfg := noGridConfig()
	cfg.StabilityWindow = 10

	trimmed, result := AutoTrim(epochs, cfg)

	if len(trimmed) != 0 {
		t.Errorf("expected 0 epochs, got %d", len(trimmed))
	}
	if result.EpochsRemoved != 15 {
		t.Errorf("expected 15 removed, got %d", result.EpochsRemoved)
	}
}

func TestAutoTrim_GridAlignment(t *testing.T) {
	// Epochs from second 10 through 100, 1-second spacing.
	// First 5 are unstable → stable start at second 15.
	// Grid ceil(15, 30) = 30.
	// Grid floor(100, 30) = 90.
	var epochs []gnss.Epoch
	for sec := 10; sec <= 100; sec++ {
		sats := 8
		snr := 40.0
		if sec < 15 {
			sats = 2 // unstable
		}
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(sec), sats, snr))
	}

	cfg := DefaultAutoTrimConfig()
	cfg.StabilityWindow = 10
	cfg.AlignToGrid = true
	cfg.GridIntervalSec = 30

	trimmed, result := AutoTrim(epochs, cfg)

	if result.TrimmedStart.TOWSeconds() != 30.0 {
		t.Errorf("expected trimmed start at 30s, got %f", result.TrimmedStart.TOWSeconds())
	}
	if result.TrimmedEnd.TOWSeconds() != 90.0 {
		t.Errorf("expected trimmed end at 90s, got %f", result.TrimmedEnd.TOWSeconds())
	}
	// All remaining epochs should be between 30 and 90 inclusive.
	for _, e := range trimmed {
		s := e.Time.TOWSeconds()
		if s < 30.0 || s > 90.0 {
			t.Errorf("epoch at %fs outside trim window [30, 90]", s)
		}
	}
	expectedCount := 90 - 30 + 1 // 61 epochs
	if len(trimmed) != expectedCount {
		t.Errorf("expected %d epochs, got %d", expectedCount, len(trimmed))
	}
}

func TestAutoTrim_GridAlreadyAligned(t *testing.T) {
	// Stable start exactly on a 30s boundary should not be shifted.
	var epochs []gnss.Epoch
	for sec := 0; sec <= 60; sec++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(sec), 8, 40.0))
	}

	cfg := DefaultAutoTrimConfig()
	cfg.StabilityWindow = 10
	cfg.AlignToGrid = true
	cfg.GridIntervalSec = 30

	_, result := AutoTrim(epochs, cfg)

	if result.TrimmedStart.TOWSeconds() != 0 {
		t.Errorf("expected trimmed start at 0s, got %f", result.TrimmedStart.TOWSeconds())
	}
	if result.TrimmedEnd.TOWSeconds() != 60.0 {
		t.Errorf("expected trimmed end at 60s, got %f", result.TrimmedEnd.TOWSeconds())
	}
}

func TestAutoTrim_StabilityWindowRequired(t *testing.T) {
	// 9 stable epochs followed by 1 unstable then 10+ stable.
	// With StabilityWindow=10, the first run of 9 should NOT qualify.
	var epochs []gnss.Epoch
	for i := 0; i < 9; i++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), 8, 40.0))
	}
	// One unstable epoch breaks the run.
	epochs = append(epochs, makeAutoTrimEpoch(2300, 9.0, 2, 40.0))
	for i := 10; i < 25; i++ {
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), 8, 40.0))
	}

	cfg := noGridConfig()
	cfg.StabilityWindow = 10

	trimmed, result := AutoTrim(epochs, cfg)

	// Stable start should be at index 10 (second 10), not index 0.
	if result.TrimmedStart.TOWSeconds() != 10.0 {
		t.Errorf("expected trimmed start at 10s, got %f", result.TrimmedStart.TOWSeconds())
	}
	// Should have trimmed the first 10 epochs (0-9).
	if result.EpochsRemoved != 10 {
		t.Errorf("expected 10 removed, got %d", result.EpochsRemoved)
	}
	if len(trimmed) != 15 {
		t.Errorf("expected 15 epochs, got %d", len(trimmed))
	}
}

func TestAutoTrim_MiddleDropout(t *testing.T) {
	// A single-epoch satellite dropout in the middle should NOT cause trimming.
	// 30 epochs, all stable except epoch 15 has only 3 sats.
	var epochs []gnss.Epoch
	for i := 0; i < 30; i++ {
		sats := 8
		if i == 15 {
			sats = 3
		}
		epochs = append(epochs, makeAutoTrimEpoch(2300, float64(i), sats, 40.0))
	}

	cfg := noGridConfig()
	cfg.StabilityWindow = 10

	trimmed, result := AutoTrim(epochs, cfg)

	// The start and end are both within the first/last 10 stable epochs,
	// so the dropout at index 15 should be inside the kept range.
	if result.EpochsRemoved != 0 {
		t.Errorf("expected 0 removed for middle dropout, got %d", result.EpochsRemoved)
	}
	if len(trimmed) != 30 {
		t.Errorf("expected 30 epochs, got %d", len(trimmed))
	}
}

func TestAutoTrim_Empty(t *testing.T) {
	trimmed, result := AutoTrim(nil, DefaultAutoTrimConfig())
	if len(trimmed) != 0 {
		t.Errorf("expected 0 epochs for nil input, got %d", len(trimmed))
	}
	if result.Reason != "no epochs provided" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}

func TestAverageGPSSNR(t *testing.T) {
	epoch := makeAutoTrimEpoch(2300, 0, 4, 35.0)
	got := averageGPSSNR(epoch)
	if got != 35.0 {
		t.Errorf("expected SNR 35.0, got %f", got)
	}

	// Mixed: add a non-GPS satellite — it should be ignored.
	epoch.Satellites = append(epoch.Satellites, gnss.SatObs{
		Constellation: gnss.ConsGLONASS,
		PRN:           1,
		Signals:       []gnss.Signal{{GnssID: 6, SNR: 10.0}},
	})
	got = averageGPSSNR(epoch)
	if got != 35.0 {
		t.Errorf("expected SNR 35.0 after adding GLONASS, got %f", got)
	}
}

func TestAverageGPSSNR_NoGPS(t *testing.T) {
	epoch := gnss.Epoch{
		Time: gnss.GNSSTime{Week: 2300, TOWNanos: 0},
		Satellites: []gnss.SatObs{
			{Constellation: gnss.ConsGLONASS, PRN: 1, Signals: []gnss.Signal{{SNR: 40.0}}},
		},
	}
	got := averageGPSSNR(epoch)
	if got != 0 {
		t.Errorf("expected 0 for no GPS signals, got %f", got)
	}
}

func TestIsStableEpoch(t *testing.T) {
	stable := makeAutoTrimEpoch(2300, 0, 8, 40.0)
	if !isStableEpoch(stable, 5, 30.0) {
		t.Error("expected stable epoch to be stable")
	}

	lowSats := makeAutoTrimEpoch(2300, 0, 3, 40.0)
	if isStableEpoch(lowSats, 5, 30.0) {
		t.Error("expected low-sat epoch to be unstable")
	}

	lowSNR := makeAutoTrimEpoch(2300, 0, 8, 25.0)
	if isStableEpoch(lowSNR, 5, 30.0) {
		t.Error("expected low-SNR epoch to be unstable")
	}
}
