package rinex

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// rinex2ObsTypes is the fixed set of RINEX 2.11 observation types we write.
// C1=L1 C/A pseudorange, C2=L2C pseudorange (civilian, never P2),
// L1/L2=carrier phase, D1/D2=Doppler, S1/S2=SNR.
var rinex2ObsTypes = []string{"C1", "C2", "L1", "L2", "D1", "D2", "S1", "S2"}

// Writer2 writes RINEX 2.11 observation files.
type Writer2 struct {
	w        io.Writer
	metadata gnss.Metadata
	arcs     map[string]*phaseArc // keyed by "G01_0" (satID_band)
}

// NewWriter2 creates a new RINEX 2.11 writer.
func NewWriter2(w io.Writer, meta gnss.Metadata) *Writer2 {
	return &Writer2{w: w, metadata: meta, arcs: make(map[string]*phaseArc)}
}

// WriteHeader writes the RINEX 2.11 header block.
func (rw *Writer2) WriteHeader() error {
	h := rw.headerLines()
	for _, line := range h {
		if _, err := fmt.Fprint(rw.w, line+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// WriteEpoch writes one epoch's observation records.
func (rw *Writer2) WriteEpoch(epoch gnss.Epoch) error {
	gpsOnly := epoch.FilterGPSOnly()
	if len(gpsOnly.Satellites) == 0 {
		return nil
	}

	epochLine := formatEpochLine(gpsOnly)
	if _, err := fmt.Fprint(rw.w, epochLine); err != nil {
		return err
	}

	for _, sat := range gpsOnly.Satellites {
		obsLine := rw.formatObsLines(sat)
		if _, err := fmt.Fprint(rw.w, obsLine); err != nil {
			return err
		}
		rw.updateArcs2(sat)
	}
	return nil
}

// WriteRinex2 is a convenience function that writes header + all epochs.
func WriteRinex2(w io.Writer, meta gnss.Metadata, epochs []gnss.Epoch) error {
	rw := NewWriter2(w, meta)
	if err := rw.WriteHeader(); err != nil {
		return err
	}
	for _, ep := range epochs {
		if err := rw.WriteEpoch(ep); err != nil {
			return err
		}
	}
	return nil
}

// --- header helpers ---

func (rw *Writer2) headerLines() []string {
	m := rw.metadata
	now := time.Now().UTC()
	var lines []string

	lines = append(lines, headerLine(
		fmt.Sprintf("%9s%-11s%-20s%-20s", "2.12", "", "OBSERVATION DATA", "G (GPS)"),
		"RINEX VERSION / TYPE"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%-20s%-20s%-20s", "RinexPrep v0.1", truncStr(m.Agency, 20),
			now.Format("20060102 15:04:05")),
		"PGM / RUN BY / DATE"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%-60s", truncStr(m.MarkerName, 60)),
		"MARKER NAME"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%-60s", truncStr(m.MarkerNumber, 20)),
		"MARKER NUMBER"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%-20s%-40s", truncStr(m.Observer, 20), truncStr(m.Agency, 40)),
		"OBSERVER / AGENCY"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%-20s%-20s%-20s", truncStr(m.ReceiverNumber, 20),
			truncStr(m.ReceiverType, 20), truncStr(m.ReceiverVersion, 20)),
		"REC # / TYPE / VERS"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%-20s%-20s%-20s", truncStr(m.AntennaNumber, 20),
			truncStr(m.AntennaType, 20), ""),
		"ANT # / TYPE"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%14.4f%14.4f%14.4f%-18s", m.ApproxX, m.ApproxY, m.ApproxZ, ""),
		"APPROX POSITION XYZ"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%14.4f%14.4f%14.4f%-18s", m.AntennaDeltaH, m.AntennaDeltaE, m.AntennaDeltaN, ""),
		"ANTENNA: DELTA H/E/N"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%-6d%-6d%-48s", 1, 1, ""),
		"WAVELENGTH FACT L1/2"))

	obsTypeLine := fmt.Sprintf("%6d", len(rinex2ObsTypes))
	for _, ot := range rinex2ObsTypes {
		obsTypeLine += fmt.Sprintf("%6s", ot)
	}
	obsTypeLine = fmt.Sprintf("%-60s", obsTypeLine)
	lines = append(lines, headerLine(obsTypeLine, "# / TYPES OF OBSERV"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%10.3f%-50s", m.Interval, ""),
		"INTERVAL"))

	fy, fmo, fd, fh, fmi, fs := GNSSTimeToCalendar(m.FirstEpoch)
	lines = append(lines, headerLine(
		fmt.Sprintf("%6d%6d%6d%6d%6d%13.7f%5s%-8s",
			fy, fmo, fd, fh, fmi, fs, "", "GPS"),
		"TIME OF FIRST OBS"))

	ly, lmo, ld, lh, lmi, ls := GNSSTimeToCalendar(m.LastEpoch)
	lines = append(lines, headerLine(
		fmt.Sprintf("%6d%6d%6d%6d%6d%13.7f%5s%-8s",
			ly, lmo, ld, lh, lmi, ls, "", "GPS"),
		"TIME OF LAST OBS"))

	lines = append(lines, headerLine(
		fmt.Sprintf("%-60s", ""),
		"END OF HEADER"))

	return lines
}

// headerLine builds an 80-character RINEX header line with right-justified label
// in columns 61-80. Shared by Writer2 and Writer3.
func headerLine(content string, label string) string {
	if len(content) > 60 {
		content = content[:60]
	}
	return fmt.Sprintf("%-60s%-20s", content, label)
}

// currentTimeStr returns the current UTC time formatted for RINEX headers.
func currentTimeStr() string {
	return time.Now().UTC().Format("20060102 15:04:05")
}

// --- epoch / observation formatting ---

// formatEpochLine builds the epoch header line(s) including satellite list.
func formatEpochLine(epoch gnss.Epoch) string {
	year, month, day, hour, min, sec := GNSSTimeToCalendar(epoch.Time)
	yy := year % 100
	nSat := len(epoch.Satellites)

	line := fmt.Sprintf(" %02d %2d %2d %2d %2d%11.7f  %d%3d",
		yy, month, day, hour, min, sec,
		epoch.Flag, nSat)

	// Satellite list: 3 chars each, max 12 per line.
	for i, sat := range epoch.Satellites {
		if i > 0 && i%12 == 0 {
			line += "\n" + strings.Repeat(" ", 32)
		}
		line += sat.SatID()
	}
	line += "\n"
	return line
}

// formatObsLines formats the observation data lines for one satellite.
// Observables order: C1, C2, L1, L2, D1, D2, S1, S2 (8 types).
func (rw *Writer2) formatObsLines(sat gnss.SatObs) string {
	l1 := bestSignalForBand(sat.Signals, 0)
	l2 := bestSignalForBand(sat.Signals, 1)

	type obsVal struct {
		value float64
		valid bool
		lli   int
		ss    int
	}

	obs := make([]obsVal, 8)

	// C1: L1 pseudorange
	if l1 != nil && l1.PRValid {
		obs[0] = obsVal{l1.Pseudorange, true, 0, snrToSS(l1.SNR)}
	}
	// C2: L2 pseudorange
	if l2 != nil && l2.PRValid {
		obs[1] = obsVal{l2.Pseudorange, true, 0, snrToSS(l2.SNR)}
	}
	// L1: carrier phase — RTKLIB-compatible LLI handling
	if l1 != nil && l1.CPValid {
		lli := rw.computeLLI(l1, sat.SatID()+"_0")
		obs[2] = obsVal{l1.CarrierPhase, true, lli, snrToSS(l1.SNR)}
	}
	// L2: carrier phase — RTKLIB-compatible LLI handling
	if l2 != nil && l2.CPValid {
		lli := rw.computeLLI(l2, sat.SatID()+"_1")
		obs[3] = obsVal{l2.CarrierPhase, true, lli, snrToSS(l2.SNR)}
	}
	// D1: Doppler
	if l1 != nil {
		obs[4] = obsVal{l1.Doppler, true, 0, 0}
	}
	// D2: Doppler
	if l2 != nil {
		obs[5] = obsVal{l2.Doppler, true, 0, 0}
	}
	// S1: SNR
	if l1 != nil && l1.SNR > 0 {
		obs[6] = obsVal{l1.SNR, true, 0, 0}
	}
	// S2: SNR
	if l2 != nil && l2.SNR > 0 {
		obs[7] = obsVal{l2.SNR, true, 0, 0}
	}

	var result string
	for i, o := range obs {
		if o.valid {
			result += fmt.Sprintf("%14.3f", o.value)
			if o.lli != 0 {
				result += fmt.Sprintf("%d", o.lli)
			} else {
				result += " "
			}
			if o.ss != 0 {
				result += fmt.Sprintf("%d", o.ss)
			} else {
				result += " "
			}
		} else {
			result += strings.Repeat(" ", 16)
		}
		// 5 observables per line
		if (i+1)%5 == 0 && i < len(obs)-1 {
			result += "\n"
		}
	}
	result += "\n"
	return result
}

// --- utility functions ---

// updateArcs2 updates carrier phase arc state for a satellite after output.
func (rw *Writer2) updateArcs2(sat gnss.SatObs) {
	for _, band := range []uint8{0, 1} {
		key := sat.SatID() + fmt.Sprintf("_%d", band)
		sig := bestSignalForBand(sat.Signals, band)
		if sig == nil {
			if arc, ok := rw.arcs[key]; ok {
				arc.present = false
			}
			continue
		}
		phaseEmitted := sig.CPValid && sig.CarrierPhase != 0
		if rw.arcs[key] == nil {
			rw.arcs[key] = &phaseArc{}
		}
		if phaseEmitted {
			rw.arcs[key].lockTime = sig.LockTimeSec
			rw.arcs[key].halfc = sig.SubHalfCyc
			rw.arcs[key].sigID = sig.SigID
		}
		rw.arcs[key].present = phaseEmitted
		rw.arcs[key].init = true
	}
}

// computeLLI returns the RTKLIB-compatible LLI value for a carrier phase signal.
func (rw *Writer2) computeLLI(sig *gnss.Signal, key string) int {
	lli := 0
	arc := rw.arcs[key]

	slip := sig.LockTimeSec == 0
	if arc != nil && arc.init {
		if sig.LockTimeSec < arc.lockTime {
			slip = true
		}
		if sig.SubHalfCyc != arc.halfc {
			slip = true
		}
	}
	if slip {
		lli |= 1
	}
	if sig.HalfCycle {
		lli |= 2
	}
	return lli
}

// bestSignalForBand returns the best signal for a given frequency band.
// For GPS L2 (band=1), prefer lower sigId (sigId=3 L2CL over sigId=4 L2CM).
func bestSignalForBand(signals []gnss.Signal, band uint8) *gnss.Signal {
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

// snrToSS maps SNR in dB-Hz to RINEX signal strength digit 1-9.
func snrToSS(snr float64) int {
	switch {
	case snr < 12:
		return 1
	case snr < 18:
		return 2
	case snr < 24:
		return 3
	case snr < 30:
		return 4
	case snr < 36:
		return 5
	case snr < 42:
		return 6
	case snr < 48:
		return 7
	case snr < 54:
		return 8
	default:
		return 9
	}
}

// truncStr truncates a string to maxLen characters.
func truncStr(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
