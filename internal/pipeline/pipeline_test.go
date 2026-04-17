package pipeline

import (
	"testing"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// makeEpoch builds a test epoch with the given number of GPS and GLONASS sats.
func makeEpoch(week uint16, towNanos int64, gpsSats, glonassSats int) gnss.Epoch {
	ep := gnss.Epoch{
		Time: gnss.GNSSTime{Week: week, TOWNanos: towNanos},
	}
	for i := 0; i < gpsSats; i++ {
		ep.Satellites = append(ep.Satellites, gnss.SatObs{
			Constellation: gnss.ConsGPS,
			PRN:           uint8(i + 1),
			Signals:       []gnss.Signal{{PRValid: true}},
		})
	}
	for i := 0; i < glonassSats; i++ {
		ep.Satellites = append(ep.Satellites, gnss.SatObs{
			Constellation: gnss.ConsGLONASS,
			PRN:           uint8(i + 1),
			Signals:       []gnss.Signal{{PRValid: true}},
		})
	}
	return ep
}

// grid30s returns the TOWNanos for a 30-second grid point n (0, 30s, 60s, ...).
func grid30s(n int) int64 {
	return int64(n) * 30_000_000_000
}

// --- Normalize tests ---

func TestNormalize_OnGrid(t *testing.T) {
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(0), 6, 0),
		makeEpoch(2300, grid30s(1), 6, 0),
		makeEpoch(2300, grid30s(2), 6, 0),
	}
	cfg := DefaultNormalizeConfig()
	result := Normalize(epochs, cfg)

	if len(result) != 3 {
		t.Fatalf("expected 3 epochs, got %d", len(result))
	}
	for i, ep := range result {
		expected := grid30s(i)
		if ep.Time.TOWNanos != expected {
			t.Errorf("epoch %d: expected TOW %d, got %d", i, expected, ep.Time.TOWNanos)
		}
	}
}

func TestNormalize_SnapWithin50ms(t *testing.T) {
	offset := int64(50_000_000) // 50ms
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(1)+offset, 6, 0),
		makeEpoch(2300, grid30s(2)-offset, 6, 0),
	}
	cfg := DefaultNormalizeConfig()
	result := Normalize(epochs, cfg)

	if len(result) != 2 {
		t.Fatalf("expected 2 epochs, got %d", len(result))
	}
	if result[0].Time.TOWNanos != grid30s(1) {
		t.Errorf("epoch 0: expected snapped to %d, got %d", grid30s(1), result[0].Time.TOWNanos)
	}
	if result[1].Time.TOWNanos != grid30s(2) {
		t.Errorf("epoch 1: expected snapped to %d, got %d", grid30s(2), result[1].Time.TOWNanos)
	}
}

func TestNormalize_DropOutOfTolerance(t *testing.T) {
	offset := int64(6_000_000_000) // 600ms, exceeds 500ms tolerance
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(0), 6, 0),
		makeEpoch(2300, grid30s(1)+offset, 6, 0),
		makeEpoch(2300, grid30s(2), 6, 0),
	}
	cfg := DefaultNormalizeConfig()
	result := Normalize(epochs, cfg)

	if len(result) != 2 {
		t.Fatalf("expected 2 epochs (1 dropped), got %d", len(result))
	}
	if result[0].Time.TOWNanos != grid30s(0) {
		t.Errorf("epoch 0: expected %d, got %d", grid30s(0), result[0].Time.TOWNanos)
	}
	if result[1].Time.TOWNanos != grid30s(2) {
		t.Errorf("epoch 1: expected %d, got %d", grid30s(2), result[1].Time.TOWNanos)
	}
}

func TestNormalize_DedupKeepClosest(t *testing.T) {
	// Two epochs near the same grid point; the closer one wins.
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(1)+80_000_000, 5, 0), // 80ms off
		makeEpoch(2300, grid30s(1)+20_000_000, 7, 0), // 20ms off — closer
	}
	cfg := DefaultNormalizeConfig()
	result := Normalize(epochs, cfg)

	if len(result) != 1 {
		t.Fatalf("expected 1 epoch after dedup, got %d", len(result))
	}
	// The closer epoch had 7 GPS sats.
	if len(result[0].Satellites) != 7 {
		t.Errorf("expected epoch with 7 sats (closer), got %d", len(result[0].Satellites))
	}
}

func TestNormalize_MonotonicOutput(t *testing.T) {
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(5), 6, 0),
		makeEpoch(2300, grid30s(1), 6, 0),
		makeEpoch(2300, grid30s(3), 6, 0),
		makeEpoch(2300, grid30s(2), 6, 0),
		makeEpoch(2300, grid30s(4), 6, 0),
	}
	cfg := DefaultNormalizeConfig()
	result := Normalize(epochs, cfg)

	for i := 1; i < len(result); i++ {
		prev := result[i-1].Time.UnixNanos()
		curr := result[i].Time.UnixNanos()
		if curr <= prev {
			t.Errorf("non-monotonic at index %d: %d >= %d", i, prev, curr)
		}
	}
}

func TestNormalize_Empty(t *testing.T) {
	result := Normalize(nil, DefaultNormalizeConfig())
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

// --- Filter tests ---

func TestFilterGPSOnly(t *testing.T) {
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(0), 5, 3), // 5 GPS + 3 GLONASS
		makeEpoch(2300, grid30s(1), 4, 2),
	}
	cfg := FilterConfig{Constellation: "gps", MinSatellites: 4}
	result := FilterConstellations(epochs, cfg)

	if len(result) != 2 {
		t.Fatalf("expected 2 epochs, got %d", len(result))
	}
	for i, ep := range result {
		for _, sat := range ep.Satellites {
			if sat.Constellation != gnss.ConsGPS {
				t.Errorf("epoch %d: found non-GPS sat %s", i, sat.SatID())
			}
		}
	}
}

func TestFilterMinSats(t *testing.T) {
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(0), 3, 5), // 3 GPS (below min=4 after filter)
		makeEpoch(2300, grid30s(1), 6, 2), // 6 GPS (above min)
	}
	cfg := FilterConfig{Constellation: "gps", MinSatellites: 4}
	result := FilterConstellations(epochs, cfg)

	if len(result) != 1 {
		t.Fatalf("expected 1 epoch (low-sat dropped), got %d", len(result))
	}
	if result[0].Time.TOWNanos != grid30s(1) {
		t.Errorf("wrong epoch kept: expected TOW %d, got %d", grid30s(1), result[0].Time.TOWNanos)
	}
}

func TestFilterAll(t *testing.T) {
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(0), 2, 3), // 5 total sats
	}
	cfg := FilterConfig{Constellation: "all", MinSatellites: 4}
	result := FilterConstellations(epochs, cfg)

	if len(result) != 1 {
		t.Fatalf("expected 1 epoch, got %d", len(result))
	}
	if len(result[0].Satellites) != 5 {
		t.Errorf("expected 5 sats with 'all' filter, got %d", len(result[0].Satellites))
	}
}

// --- Trim tests ---

func TestTrim_WindowApplied(t *testing.T) {
	start := gnss.GNSSTime{Week: 2300, TOWNanos: grid30s(2)}
	end := gnss.GNSSTime{Week: 2300, TOWNanos: grid30s(4)}
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(1), 6, 0),
		makeEpoch(2300, grid30s(2), 6, 0),
		makeEpoch(2300, grid30s(3), 6, 0),
		makeEpoch(2300, grid30s(4), 6, 0),
		makeEpoch(2300, grid30s(5), 6, 0),
	}
	cfg := TrimConfig{Start: &start, End: &end}
	result := Trim(epochs, cfg)

	if len(result) != 3 {
		t.Fatalf("expected 3 epochs in window, got %d", len(result))
	}
	if result[0].Time.TOWNanos != grid30s(2) {
		t.Errorf("first epoch: expected %d, got %d", grid30s(2), result[0].Time.TOWNanos)
	}
	if result[2].Time.TOWNanos != grid30s(4) {
		t.Errorf("last epoch: expected %d, got %d", grid30s(4), result[2].Time.TOWNanos)
	}
}

func TestTrim_NilBounds(t *testing.T) {
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(0), 6, 0),
		makeEpoch(2300, grid30s(1), 6, 0),
	}
	result := Trim(epochs, TrimConfig{})

	if len(result) != 2 {
		t.Fatalf("nil bounds should keep all, got %d", len(result))
	}
}

func TestTrim_StartOnly(t *testing.T) {
	start := gnss.GNSSTime{Week: 2300, TOWNanos: grid30s(3)}
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(1), 6, 0),
		makeEpoch(2300, grid30s(3), 6, 0),
		makeEpoch(2300, grid30s(5), 6, 0),
	}
	result := Trim(epochs, TrimConfig{Start: &start})

	if len(result) != 2 {
		t.Fatalf("expected 2 epochs, got %d", len(result))
	}
}

func TestTrim_Empty(t *testing.T) {
	result := Trim(nil, TrimConfig{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

// --- Pipeline end-to-end tests ---

func TestProcess_EndToEnd(t *testing.T) {
	offset50ms := int64(50_000_000)
	offset600ms := int64(6_000_000_000) // exceeds 500ms tolerance

	start := gnss.GNSSTime{Week: 2300, TOWNanos: grid30s(1)}
	end := gnss.GNSSTime{Week: 2300, TOWNanos: grid30s(5)}

	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(0), 6, 2),                // trimmed (before start)
		makeEpoch(2300, grid30s(1)+offset50ms, 5, 3),      // kept, snapped to grid 1
		makeEpoch(2300, grid30s(2)+offset600ms, 6, 0),     // dropped by normalize (off-grid)
		makeEpoch(2300, grid30s(3), 3, 5),                 // dropped by filter (3 GPS < 4)
		makeEpoch(2300, grid30s(4), 6, 0),                 // kept
		makeEpoch(2300, grid30s(5), 6, 0),                 // kept
		makeEpoch(2300, grid30s(6), 6, 0),                 // trimmed (after end)
	}

	cfg := DefaultConfig()
	cfg.Trim = TrimConfig{Start: &start, End: &end}

	result, stats := Process(epochs, cfg)

	if stats.InputEpochs != 7 {
		t.Errorf("InputEpochs: expected 7, got %d", stats.InputEpochs)
	}
	if stats.AfterTrim != 5 {
		t.Errorf("AfterTrim: expected 5, got %d", stats.AfterTrim)
	}
	if stats.AfterFilter != 4 {
		t.Errorf("AfterFilter: expected 4, got %d", stats.AfterFilter)
	}
	if stats.AfterNormalize != 3 {
		t.Errorf("AfterNormalize: expected 3, got %d", stats.AfterNormalize)
	}
	if stats.DroppedOffGrid != 1 {
		t.Errorf("DroppedOffGrid: expected 1, got %d", stats.DroppedOffGrid)
	}
	if stats.DroppedLowSats != 1 {
		t.Errorf("DroppedLowSats: expected 1, got %d", stats.DroppedLowSats)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 final epochs, got %d", len(result))
	}

	// Verify monotonic
	for i := 1; i < len(result); i++ {
		if result[i].Time.UnixNanos() <= result[i-1].Time.UnixNanos() {
			t.Errorf("non-monotonic at index %d", i)
		}
	}
}

func TestStats_DefaultNoTrim(t *testing.T) {
	epochs := []gnss.Epoch{
		makeEpoch(2300, grid30s(0), 6, 0),
		makeEpoch(2300, grid30s(1), 6, 0),
	}
	cfg := DefaultConfig()
	result, stats := Process(epochs, cfg)

	if stats.InputEpochs != 2 || stats.AfterTrim != 2 || stats.AfterFilter != 2 || stats.AfterNormalize != 2 {
		t.Errorf("unexpected stats for clean input: %+v", stats)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 epochs, got %d", len(result))
	}
}

func TestProcess_Empty(t *testing.T) {
	result, stats := Process(nil, DefaultConfig())
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
	if stats.InputEpochs != 0 {
		t.Errorf("expected 0 input epochs, got %d", stats.InputEpochs)
	}
}
