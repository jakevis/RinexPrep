package gnss

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// TOWSeconds
// ---------------------------------------------------------------------------

func TestTOWSeconds_Zero(t *testing.T) {
	gt := GNSSTime{TOWNanos: 0}
	if got := gt.TOWSeconds(); got != 0 {
		t.Errorf("TOWSeconds() = %v, want 0", got)
	}
}

func TestTOWSeconds_MidWeek(t *testing.T) {
	// Wednesday 00:00:00 = 3 days = 259200 s
	nanos := int64(259200) * 1e9
	gt := GNSSTime{TOWNanos: nanos}
	if got := gt.TOWSeconds(); got != 259200.0 {
		t.Errorf("TOWSeconds() = %v, want 259200", got)
	}
}

func TestTOWSeconds_345600(t *testing.T) {
	// 345600 s = 4 days = Thursday 00:00:00
	nanos := int64(345600) * 1e9
	gt := GNSSTime{TOWNanos: nanos}
	if got := gt.TOWSeconds(); got != 345600.0 {
		t.Errorf("TOWSeconds() = %v, want 345600", got)
	}
}

func TestTOWSeconds_EndOfWeek(t *testing.T) {
	// 604799.999999999 s – last nanosecond of the week
	nanos := int64(604799999999999)
	gt := GNSSTime{TOWNanos: nanos}
	want := 604799.999999999
	got := gt.TOWSeconds()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("TOWSeconds() = %v, want ~%v", got, want)
	}
}

func TestTOWSeconds_NanosecondPrecision(t *testing.T) {
	// A typical GNSS epoch: 345600.123456789 s
	nanos := int64(345600123456789)
	gt := GNSSTime{TOWNanos: nanos}
	got := gt.TOWSeconds()
	// float64 has ~15-16 significant digits; 345600.123456789 has 15 digits,
	// so we allow a tiny tolerance.
	want := 345600.123456789
	if math.Abs(got-want) > 1e-4 {
		t.Errorf("TOWSeconds() = %v, want ~%v (within 100µs)", got, want)
	}
}

// ---------------------------------------------------------------------------
// UnixNanos
// ---------------------------------------------------------------------------

func TestUnixNanos_GPSEpoch(t *testing.T) {
	gt := GNSSTime{Week: 0, TOWNanos: 0}
	if got := gt.UnixNanos(); got != GPSEpochUnixNanos {
		t.Errorf("UnixNanos() = %d, want GPSEpochUnixNanos = %d", got, GPSEpochUnixNanos)
	}
}

func TestUnixNanos_GPSEpochValue(t *testing.T) {
	// GPS epoch = Jan 6, 1980 00:00:00 UTC = 315964800 Unix seconds.
	wantUnix := int64(315964800) * 1e9
	if GPSEpochUnixNanos != wantUnix {
		t.Errorf("GPSEpochUnixNanos = %d, want %d", GPSEpochUnixNanos, wantUnix)
	}
}

func TestUnixNanos_Arithmetic(t *testing.T) {
	week := uint16(2356)
	tow := int64(345600) * 1e9 // Thursday 00:00
	gt := GNSSTime{Week: week, TOWNanos: tow}

	want := GPSEpochUnixNanos + int64(week)*7*24*3600*1e9 + tow
	if got := gt.UnixNanos(); got != want {
		t.Errorf("UnixNanos() = %d, want %d", got, want)
	}
}

func TestUnixNanos_Week1_TOW0(t *testing.T) {
	gt := GNSSTime{Week: 1, TOWNanos: 0}
	want := GPSEpochUnixNanos + 7*24*3600*int64(1e9)
	if got := gt.UnixNanos(); got != want {
		t.Errorf("UnixNanos() = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// UTCUnixNanos
// ---------------------------------------------------------------------------

func TestUTCUnixNanos_ValidLeap(t *testing.T) {
	gt := GNSSTime{
		Week:        2356,
		TOWNanos:    int64(345600) * 1e9,
		LeapSeconds: 18,
		LeapValid:   true,
	}
	gps := gt.UnixNanos()
	want := gps - 18*int64(1e9)
	if got := gt.UTCUnixNanos(); got != want {
		t.Errorf("UTCUnixNanos() = %d, want %d (GPS - 18s)", got, want)
	}
}

func TestUTCUnixNanos_InvalidLeap(t *testing.T) {
	gt := GNSSTime{
		Week:        2356,
		TOWNanos:    int64(345600) * 1e9,
		LeapSeconds: 18,
		LeapValid:   false,
	}
	if got := gt.UTCUnixNanos(); got != 0 {
		t.Errorf("UTCUnixNanos() = %d, want 0 when LeapValid=false", got)
	}
}

func TestUTCUnixNanos_ZeroLeap(t *testing.T) {
	gt := GNSSTime{
		Week:        0,
		TOWNanos:    0,
		LeapSeconds: 0,
		LeapValid:   true,
	}
	// With 0 leap seconds, UTC == GPS.
	if got := gt.UTCUnixNanos(); got != gt.UnixNanos() {
		t.Errorf("UTCUnixNanos() = %d, want %d (same as GPS with 0 leap)", got, gt.UnixNanos())
	}
}

// ---------------------------------------------------------------------------
// GridOffset30s
// ---------------------------------------------------------------------------

func TestGridOffset30s_OnGrid(t *testing.T) {
	cases := []int64{0, 30e9, 60e9, 300e9, 604800e9 - 30e9}
	for _, tow := range cases {
		gt := GNSSTime{TOWNanos: tow}
		if off := gt.GridOffset30s(); off != 0 {
			t.Errorf("GridOffset30s() for TOW=%d = %d, want 0", tow, off)
		}
	}
}

func TestGridOffset30s_SlightlyAfter(t *testing.T) {
	// 50ms after a 30s grid point
	gt := GNSSTime{TOWNanos: 30e9 + 50_000_000} // 30.050s
	off := gt.GridOffset30s()
	if off != 50_000_000 {
		t.Errorf("GridOffset30s() = %d, want 50000000 (50ms)", off)
	}
}

func TestGridOffset30s_SlightlyBefore(t *testing.T) {
	// 50ms before the next 30s grid point → -50ms
	gt := GNSSTime{TOWNanos: 60e9 - 50_000_000} // 59.950s
	off := gt.GridOffset30s()
	if off != -50_000_000 {
		t.Errorf("GridOffset30s() = %d, want -50000000 (-50ms)", off)
	}
}

func TestGridOffset30s_ExactlyHalfway(t *testing.T) {
	// 15s into a 30s window → mod=15e9, which equals grid30s/2.
	// Since mod is NOT > grid30s/2, it returns mod directly (positive 15s).
	gt := GNSSTime{TOWNanos: 15e9}
	off := gt.GridOffset30s()
	if off != 15e9 {
		t.Errorf("GridOffset30s() = %d, want %d (+15s)", off, int64(15e9))
	}
}

func TestGridOffset30s_JustPastHalfway(t *testing.T) {
	// 15s + 1ns → should be negative (closer to next grid point)
	gt := GNSSTime{TOWNanos: 15_000_000_001}
	off := gt.GridOffset30s()
	want := int64(15_000_000_001) - 30e9
	if off != want {
		t.Errorf("GridOffset30s() = %d, want %d", off, want)
	}
}

func TestGridOffset30s_NegativeTOW(t *testing.T) {
	// Negative TOW (could happen transiently before normalization)
	gt := GNSSTime{TOWNanos: -50_000_000} // -50ms
	off := gt.GridOffset30s()
	// mod = -50ms % 30s = negative, then += 30s → 29.95s.
	// 29.95s > 15s → returns 29.95s - 30s = -50ms
	if off != -50_000_000 {
		t.Errorf("GridOffset30s() for negative TOW = %d, want -50000000", off)
	}
}

func TestGridOffset30s_VariousPositions(t *testing.T) {
	tests := []struct {
		name    string
		towNs   int64
		wantOff int64
	}{
		{"1s after grid", 1e9, 1e9},
		{"10s after grid", 10e9, 10e9},
		{"20s after grid", 50e9, -10e9},    // 50s → mod=20s (after 30s grid) → 20>15 → 20-30=-10s
		{"29s after grid", 29e9, -1e9},      // 29s → mod=29s → 29>15 → 29-30=-1s
		{"exactly 30s", 30e9, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt := GNSSTime{TOWNanos: tt.towNs}
			if got := gt.GridOffset30s(); got != tt.wantOff {
				t.Errorf("GridOffset30s() = %d, want %d", got, tt.wantOff)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SnapToGrid30s
// ---------------------------------------------------------------------------

func TestSnapToGrid30s_AlreadyOnGrid(t *testing.T) {
	gt := GNSSTime{Week: 100, TOWNanos: 60e9}
	snapped := gt.SnapToGrid30s()
	if snapped.Week != gt.Week || snapped.TOWNanos != gt.TOWNanos {
		t.Errorf("SnapToGrid30s changed on-grid time: got week=%d tow=%d, want week=%d tow=%d",
			snapped.Week, snapped.TOWNanos, gt.Week, gt.TOWNanos)
	}
}

func TestSnapToGrid30s_SlightlyAfter(t *testing.T) {
	// 30.050s → should snap to 30s
	gt := GNSSTime{Week: 100, TOWNanos: 30e9 + 50_000_000}
	snapped := gt.SnapToGrid30s()
	if snapped.TOWNanos != 30e9 || snapped.Week != 100 {
		t.Errorf("SnapToGrid30s() = week=%d tow=%d, want week=100 tow=%d",
			snapped.Week, snapped.TOWNanos, int64(30e9))
	}
}

func TestSnapToGrid30s_SlightlyBeforeNext(t *testing.T) {
	// 59.950s → should snap forward to 60s
	gt := GNSSTime{Week: 100, TOWNanos: 60e9 - 50_000_000}
	snapped := gt.SnapToGrid30s()
	if snapped.TOWNanos != 60e9 || snapped.Week != 100 {
		t.Errorf("SnapToGrid30s() = week=%d tow=%d, want week=100 tow=%d",
			snapped.Week, snapped.TOWNanos, int64(60e9))
	}
}

func TestSnapToGrid30s_WeekBoundaryBackward(t *testing.T) {
	// TOW near 0, offset is positive (slightly after 0), should snap to 0.
	gt := GNSSTime{Week: 100, TOWNanos: 50_000_000} // 50ms
	snapped := gt.SnapToGrid30s()
	if snapped.TOWNanos != 0 || snapped.Week != 100 {
		t.Errorf("SnapToGrid30s() = week=%d tow=%d, want week=100 tow=0",
			snapped.Week, snapped.TOWNanos)
	}
}

func TestSnapToGrid30s_WeekBoundaryBackwardCross(t *testing.T) {
	// TOW is negative (e.g. -50ms after some arithmetic) → snap should cross
	// week boundary: week decrements, TOW wraps to end of previous week.
	gt := GNSSTime{Week: 100, TOWNanos: -50_000_000}
	// GridOffset30s for -50ms → -50ms, so snapped TOW = -50ms - (-50ms) = 0
	// Actually let's trace: mod = -50ms % 30e9. In Go, this is negative.
	// mod = -50_000_000 % 30_000_000_000 = -50_000_000
	// mod < 0 → mod += 30e9 = 29_950_000_000
	// 29_950_000_000 > 15e9 → return 29_950_000_000 - 30e9 = -50_000_000
	// offset = -50ms
	// snapped TOW = -50ms - (-50ms) = 0, week = 100
	snapped := gt.SnapToGrid30s()
	if snapped.TOWNanos != 0 || snapped.Week != 100 {
		t.Errorf("SnapToGrid30s() = week=%d tow=%d, want week=100 tow=0",
			snapped.Week, snapped.TOWNanos)
	}
}

func TestSnapToGrid30s_WeekBoundaryForward(t *testing.T) {
	// TOW near end of week, snaps forward past the week boundary.
	weekNanos := int64(7*24*3600) * 1e9
	gt := GNSSTime{Week: 100, TOWNanos: weekNanos - 50_000_000} // 50ms before end
	// Offset = -50ms → snapped TOW = (weekNanos - 50ms) - (-50ms) = weekNanos
	// weekNanos >= weekNanos → wraps: TOW=0, week=101
	snapped := gt.SnapToGrid30s()
	if snapped.TOWNanos != 0 || snapped.Week != 101 {
		t.Errorf("SnapToGrid30s() = week=%d tow=%d, want week=101 tow=0",
			snapped.Week, snapped.TOWNanos)
	}
}

func TestSnapToGrid30s_Idempotent(t *testing.T) {
	cases := []GNSSTime{
		{Week: 100, TOWNanos: 30e9 + 50_000_000},
		{Week: 200, TOWNanos: 60e9 - 50_000_000},
		{Week: 50, TOWNanos: 12345678},
		{Week: 0, TOWNanos: 0},
	}
	for _, gt := range cases {
		s1 := gt.SnapToGrid30s()
		s2 := s1.SnapToGrid30s()
		if s1.Week != s2.Week || s1.TOWNanos != s2.TOWNanos {
			t.Errorf("snap not idempotent: snap1=(%d,%d), snap2=(%d,%d)",
				s1.Week, s1.TOWNanos, s2.Week, s2.TOWNanos)
		}
	}
}

func TestSnapToGrid30s_PreservesFields(t *testing.T) {
	gt := GNSSTime{
		Week:        100,
		TOWNanos:    30e9 + 50_000_000,
		TimeSystem:  TimeGalileo,
		LeapSeconds: 18,
		LeapValid:   true,
		ClkReset:    true,
	}
	snapped := gt.SnapToGrid30s()
	if snapped.ClkReset != true {
		t.Error("SnapToGrid30s lost ClkReset")
	}
	if snapped.LeapSeconds != 18 {
		t.Errorf("SnapToGrid30s changed LeapSeconds: got %d, want 18", snapped.LeapSeconds)
	}
	if snapped.LeapValid != true {
		t.Error("SnapToGrid30s lost LeapValid")
	}
	if snapped.TimeSystem != TimeGalileo {
		t.Error("SnapToGrid30s changed TimeSystem")
	}
}
