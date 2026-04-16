package rinex

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// rinex3ObsCodes defines the default observation codes per constellation.
var rinex3ObsCodes = map[gnss.Constellation][]string{
	gnss.ConsGPS:     {"C1C", "L1C", "D1C", "S1C", "C2L", "L2L", "D2L", "S2L"},
	gnss.ConsGLONASS: {"C1C", "L1C", "D1C", "S1C", "C2C", "L2C", "D2C", "S2C"},
	gnss.ConsGalileo: {"C1C", "L1C", "D1C", "S1C", "C7I", "L7I", "D7I", "S7I"},
	gnss.ConsBeiDou:  {"C2I", "L2I", "D2I", "S2I", "C7I", "L7I", "D7I", "S7I"},
}

// constellationOrder is the canonical order for writing systems in the header.
var constellationOrder = []gnss.Constellation{
	gnss.ConsGPS,
	gnss.ConsGLONASS,
	gnss.ConsGalileo,
	gnss.ConsBeiDou,
}

// Writer3 writes RINEX 3.04 observation files.
type Writer3 struct {
	w        io.Writer
	metadata gnss.Metadata
	gpsOnly  bool
}

// NewWriter3 creates a new RINEX 3.04 writer.
func NewWriter3(w io.Writer, meta gnss.Metadata) *Writer3 {
	return &Writer3{
		w:        w,
		metadata: meta,
	}
}

// detectSystems scans epochs and returns which constellations are present.
func detectSystems(epochs []gnss.Epoch) map[gnss.Constellation]bool {
	systems := make(map[gnss.Constellation]bool)
	for i := range epochs {
		for j := range epochs[i].Satellites {
			systems[epochs[i].Satellites[j].Constellation] = true
		}
	}
	return systems
}

// isGPSOnly returns true if only GPS satellites are present in the epochs.
func isGPSOnly(epochs []gnss.Epoch) bool {
	for i := range epochs {
		for j := range epochs[i].Satellites {
			if epochs[i].Satellites[j].Constellation != gnss.ConsGPS {
				return false
			}
		}
	}
	return true
}

// WriteHeader writes the RINEX 3.04 header.
func (rw *Writer3) WriteHeader() error {
	return rw.writeHeaderWithSystems(nil)
}

// writeHeaderWithSystems writes the header, using the given system set for
// SYS / # / OBS TYPES lines. If systems is nil, all four default constellations
// are written.
func (rw *Writer3) writeHeaderWithSystems(systems map[gnss.Constellation]bool) error {
	m := rw.metadata
	var lines []string

	// RINEX VERSION / TYPE
	sysChar := "M (MIXED)"
	if rw.gpsOnly {
		sysChar = "G (GPS)"
	}
	versionLine := fmt.Sprintf("%9s%11s%-20s%-20s", "3.04", "", "OBSERVATION DATA", sysChar)
	lines = append(lines, headerLine(versionLine, "RINEX VERSION / TYPE"))

	// PGM / RUN BY / DATE
	now := time.Now().UTC()
	pgmLine := fmt.Sprintf("%-20s%-20s%-20s",
		truncStr("RinexPrep v0.1", 20),
		truncStr(m.Agency, 20),
		now.Format("20060102 15:04:05"),
	)
	lines = append(lines, headerLine(pgmLine, "PGM / RUN BY / DATE"))

	// MARKER NAME
	lines = append(lines, headerLine(truncStr(m.MarkerName, 60), "MARKER NAME"))

	// MARKER NUMBER
	lines = append(lines, headerLine(truncStr(m.MarkerNumber, 60), "MARKER NUMBER"))

	// OBSERVER / AGENCY
	obsLine := fmt.Sprintf("%-20s%-40s",
		truncStr(m.Observer, 20),
		truncStr(m.Agency, 40),
	)
	lines = append(lines, headerLine(obsLine, "OBSERVER / AGENCY"))

	// REC # / TYPE / VERS
	recLine := fmt.Sprintf("%-20s%-20s%-20s",
		truncStr(m.ReceiverNumber, 20),
		truncStr(m.ReceiverType, 20),
		truncStr(m.ReceiverVersion, 20),
	)
	lines = append(lines, headerLine(recLine, "REC # / TYPE / VERS"))

	// ANT # / TYPE
	antLine := fmt.Sprintf("%-20s%-40s",
		truncStr(m.AntennaNumber, 20),
		truncStr(m.AntennaType, 40),
	)
	lines = append(lines, headerLine(antLine, "ANT # / TYPE"))

	// APPROX POSITION XYZ
	posLine := fmt.Sprintf("%14.4f%14.4f%14.4f%18s",
		m.ApproxX, m.ApproxY, m.ApproxZ, "")
	lines = append(lines, headerLine(posLine, "APPROX POSITION XYZ"))

	// ANTENNA: DELTA H/E/N
	deltaLine := fmt.Sprintf("%14.4f%14.4f%14.4f%18s",
		m.AntennaDeltaH, m.AntennaDeltaE, m.AntennaDeltaN, "")
	lines = append(lines, headerLine(deltaLine, "ANTENNA: DELTA H/E/N"))

	// SYS / # / OBS TYPES
	for _, cons := range constellationOrder {
		if systems != nil && !systems[cons] {
			continue
		}
		codes := rinex3ObsCodes[cons]
		sysLine := fmt.Sprintf("%s  %3d", cons.String(), len(codes))
		for _, code := range codes {
			sysLine += fmt.Sprintf(" %s", code)
		}
		lines = append(lines, headerLine(sysLine, "SYS / # / OBS TYPES"))
	}

	// INTERVAL
	intervalLine := fmt.Sprintf("%10.3f%50s", m.Interval, "")
	lines = append(lines, headerLine(intervalLine, "INTERVAL"))

	// TIME OF FIRST OBS
	lines = append(lines, rw.formatTimeHeaderLine(m.FirstEpoch, "TIME OF FIRST OBS"))

	// TIME OF LAST OBS
	lines = append(lines, rw.formatTimeHeaderLine(m.LastEpoch, "TIME OF LAST OBS"))

	// END OF HEADER
	lines = append(lines, headerLine("", "END OF HEADER"))

	for _, line := range lines {
		if _, err := fmt.Fprintln(rw.w, line); err != nil {
			return err
		}
	}
	return nil
}

func (rw *Writer3) formatTimeHeaderLine(t gnss.GNSSTime, label string) string {
	year, month, day, hour, min, sec := GNSSTimeToCalendar(t)
	timeLine := fmt.Sprintf("  %4d  %2d  %2d  %2d  %2d %13.7f     GPS         ",
		year, month, day, hour, min, sec)
	return headerLine(timeLine, label)
}

// WriteEpoch writes one epoch in RINEX 3 format.
func (rw *Writer3) WriteEpoch(epoch gnss.Epoch) error {
	year, month, day, hour, min, sec := GNSSTimeToCalendar(epoch.Time)

	epochLine := fmt.Sprintf("> %4d %2d %2d %2d %2d%11.7f  %d%3d",
		year, month, day, hour, min, sec,
		epoch.Flag, len(epoch.Satellites))
	if _, err := fmt.Fprintln(rw.w, epochLine); err != nil {
		return err
	}

	// Sort satellites by constellation then PRN for deterministic output
	sats := make([]gnss.SatObs, len(epoch.Satellites))
	copy(sats, epoch.Satellites)
	sort.Slice(sats, func(i, j int) bool {
		if sats[i].Constellation != sats[j].Constellation {
			return sats[i].Constellation < sats[j].Constellation
		}
		return sats[i].PRN < sats[j].PRN
	})

	for _, sat := range sats {
		codes := rinex3ObsCodes[sat.Constellation]
		if codes == nil {
			continue
		}

		var line strings.Builder
		line.WriteString(sat.SatID())

		for _, code := range codes {
			val, lli, ss := resolveObs3(sat, code)
			if val == 0 {
				line.WriteString(strings.Repeat(" ", 16))
			} else {
				line.WriteString(fmt.Sprintf("%14.3f%c%c", val, lli, ss))
			}
		}

		if _, err := fmt.Fprintln(rw.w, line.String()); err != nil {
			return err
		}
	}
	return nil
}

// resolveObs3 returns the value, LLI flag, and signal strength for a given
// RINEX 3 observation code from a satellite's signals.
func resolveObs3(sat gnss.SatObs, code string) (val float64, lli byte, ss byte) {
	lli = ' '
	ss = ' '

	if len(code) < 3 {
		return
	}

	obsType := code[0]  // C, L, D, S
	freqChar := code[1] // '1', '2', '5', '7'

	var targetBand uint8
	switch freqChar {
	case '1':
		targetBand = 0
	case '2':
		targetBand = 1
	case '5':
		targetBand = 2
	case '7':
		targetBand = 2 // E5b / B2 maps to FreqBand 2
	default:
		return
	}

	sig := bestSignalForBand(sat.Signals, targetBand)
	if sig == nil {
		return
	}

	switch obsType {
	case 'C':
		if sig.PRValid && sig.Pseudorange != 0 {
			val = sig.Pseudorange
		}
	case 'L':
		if sig.CPValid && sig.CarrierPhase != 0 {
			val = sig.CarrierPhase
			if sig.HalfCycle {
				lli = '1'
			}
		}
	case 'D':
		if sig.Doppler != 0 {
			val = sig.Doppler
		}
	case 'S':
		if sig.SNR != 0 {
			val = sig.SNR
			ss = byte('0' + snrToSS(sig.SNR))
		}
	}
	return
}

// WriteRinex3 is a convenience function for complete file output.
func WriteRinex3(w io.Writer, meta gnss.Metadata, epochs []gnss.Epoch) error {
	rw := NewWriter3(w, meta)
	rw.gpsOnly = isGPSOnly(epochs)

	systems := detectSystems(epochs)
	if err := rw.writeHeaderWithSystems(systems); err != nil {
		return err
	}
	for i := range epochs {
		if err := rw.WriteEpoch(epochs[i]); err != nil {
			return err
		}
	}
	return nil
}
