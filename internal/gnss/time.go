package gnss

// GNSSTime is the canonical time representation for all GNSS observations.
// We store GPS week + nanosecond TOW to avoid floating-point drift and
// to preserve the native time system from the receiver.
type GNSSTime struct {
	// Week is the GPS week number (continuous, not modulo 1024).
	Week uint16

	// TOWNanos is the time of week in nanoseconds from the start of
	// the GPS week (Sunday 00:00:00 GPS time).
	TOWNanos int64

	// TimeSystem indicates the time system (GPS, GLONASS, Galileo, BeiDou).
	TimeSystem TimeSystem

	// LeapSeconds is the number of leap seconds between GPS and UTC.
	// If LeapValid is false, this value should not be trusted.
	LeapSeconds int8

	// LeapValid indicates whether the receiver has confirmed the
	// leap second count (from almanac or navigation message).
	LeapValid bool

	// ClkReset indicates a receiver clock reset occurred at this epoch.
	ClkReset bool
}

// TimeSystem identifies the GNSS time reference.
type TimeSystem uint8

const (
	TimeGPS     TimeSystem = 0
	TimeGLONASS TimeSystem = 1
	TimeGalileo TimeSystem = 2
	TimeBeiDou  TimeSystem = 3
	TimeUTC     TimeSystem = 7
)

// GPSEpoch is the GPS time origin: January 6, 1980 00:00:00 UTC.
const GPSEpochUnixNanos int64 = 315964800_000_000_000

// TOWSeconds returns the time of week as a float64 in seconds.
func (t GNSSTime) TOWSeconds() float64 {
	return float64(t.TOWNanos) / 1e9
}

// UnixNanos converts GNSSTime to Unix nanoseconds (GPS timescale, no leap seconds).
func (t GNSSTime) UnixNanos() int64 {
	weekNanos := int64(t.Week) * 7 * 24 * 3600 * 1e9
	return GPSEpochUnixNanos + weekNanos + t.TOWNanos
}

// UTCUnixNanos converts GNSSTime to UTC Unix nanoseconds (applies leap seconds).
// Returns 0 if leap seconds are not valid.
func (t GNSSTime) UTCUnixNanos() int64 {
	if !t.LeapValid {
		return 0
	}
	return t.UnixNanos() - int64(t.LeapSeconds)*1e9
}

// GridOffset30s returns the offset in nanoseconds from the nearest 30-second grid point.
// Positive means the epoch is after the grid point, negative means before.
func (t GNSSTime) GridOffset30s() int64 {
	const grid30s int64 = 30_000_000_000
	mod := t.TOWNanos % grid30s
	if mod < 0 {
		mod += grid30s
	}
	if mod > grid30s/2 {
		return mod - grid30s
	}
	return mod
}

// SnapToGrid30s returns a new GNSSTime snapped to the nearest 30-second boundary.
func (t GNSSTime) SnapToGrid30s() GNSSTime {
	const grid30s int64 = 30_000_000_000
	offset := t.GridOffset30s()
	snapped := t
	snapped.TOWNanos = t.TOWNanos - offset
	// Normalize if snapping crosses week boundary
	if snapped.TOWNanos < 0 {
		snapped.TOWNanos += 7 * 24 * 3600 * 1e9
		snapped.Week--
	}
	secsInWeek := int64(7 * 24 * 3600) * 1e9
	if snapped.TOWNanos >= secsInWeek {
		snapped.TOWNanos -= secsInWeek
		snapped.Week++
	}
	return snapped
}
