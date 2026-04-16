package rinex

import (
	"time"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// gpsEpoch is the GPS time origin: January 6, 1980 00:00:00 UTC.
var gpsEpoch = time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

// GNSSTimeToCalendar converts GNSSTime to calendar components.
func GNSSTimeToCalendar(t gnss.GNSSTime) (year, month, day, hour, min int, sec float64) {
	weekDur := time.Duration(int64(t.Week)) * 7 * 24 * time.Hour
	towDur := time.Duration(t.TOWNanos) * time.Nanosecond
	ct := gpsEpoch.Add(weekDur + towDur)

	year = ct.Year()
	month = int(ct.Month())
	day = ct.Day()
	hour = ct.Hour()
	min = ct.Minute()
	sec = float64(ct.Second()) + float64(ct.Nanosecond())/1e9
	return
}
