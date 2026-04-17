package rinex

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jakevis/rinexprep/internal/gnss"
)

func testMetadata2() gnss.Metadata {
	return gnss.Metadata{
		MarkerName:      "TEST0001",
		MarkerNumber:    "1234",
		ReceiverNumber:  "REC001",
		ReceiverType:    "ZED-F9P",
		ReceiverVersion: "1.32",
		AntennaNumber:   "ANT001",
		AntennaType:     "ADVNULLANTENNA",
		AntennaDeltaH:   1.5000,
		AntennaDeltaE:   0.0000,
		AntennaDeltaN:   0.0000,
		ApproxX:         -2694892.4600,
		ApproxY:         -4297439.0200,
		ApproxZ:         3854263.1100,
		Observer:        "TestObs",
		Agency:          "TestAgency",
		Interval:        30.0,
		FirstEpoch: gnss.GNSSTime{
			Week:     2345,
			TOWNanos: 259200_000_000_000, // Wednesday 00:00:00
		},
		LastEpoch: gnss.GNSSTime{
			Week:     2345,
			TOWNanos: 345600_000_000_000, // Thursday 00:00:00
		},
	}
}

func makeEpoch(week uint16, towNanos int64, sats []gnss.SatObs) gnss.Epoch {
	return gnss.Epoch{
		Time: gnss.GNSSTime{
			Week:     week,
			TOWNanos: towNanos,
		},
		Flag:       0,
		Satellites: sats,
	}
}

func makeSatObs(prn uint8, signals []gnss.Signal) gnss.SatObs {
	return gnss.SatObs{
		Constellation: gnss.ConsGPS,
		PRN:           prn,
		Signals:       signals,
	}
}

func TestWriteHeader(t *testing.T) {
	var buf bytes.Buffer
	meta := testMetadata2()
	rw := NewWriter2(&buf, meta)
	if err := rw.WriteHeader(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Each header line must be 80 chars
	for i, line := range lines {
		if len(line) != 80 {
			t.Errorf("header line %d length = %d, want 80: %q", i, len(line), line)
		}
	}

	// Check RINEX VERSION / TYPE
	if !strings.Contains(lines[0], "2.12") {
		t.Error("first line missing version 2.12")
	}
	if !strings.Contains(lines[0], "RINEX VERSION / TYPE") {
		t.Error("first line missing RINEX VERSION / TYPE label")
	}
	if !strings.Contains(lines[0], "OBSERVATION DATA") {
		t.Error("first line missing OBSERVATION DATA")
	}
	if !strings.Contains(lines[0], "G (GPS)") {
		t.Error("first line missing G (GPS)")
	}

	// Check PGM / RUN BY / DATE
	if !strings.Contains(lines[1], "RinexPrep v0.1") {
		t.Error("line 2 missing PGM")
	}
	if !strings.Contains(lines[1], "PGM / RUN BY / DATE") {
		t.Error("line 2 missing label")
	}

	// Check MARKER NAME
	if !strings.Contains(lines[2], "TEST0001") {
		t.Error("MARKER NAME missing")
	}

	// Check obs types contain C2 not P2
	obsLine := ""
	for _, l := range lines {
		if strings.Contains(l, "# / TYPES OF OBSERV") {
			obsLine = l
			break
		}
	}
	if obsLine == "" {
		t.Fatal("# / TYPES OF OBSERV not found")
	}
	if !strings.Contains(obsLine, "C2") {
		t.Error("obs types should contain C2")
	}
	if strings.Contains(obsLine, "P2") {
		t.Error("obs types should NOT contain P2")
	}
	if !strings.Contains(obsLine, "C1") {
		t.Error("obs types should contain C1")
	}
	// Check count
	if !strings.Contains(obsLine, "     8") {
		t.Errorf("obs type count should be 8, got: %q", obsLine[:6])
	}

	// Check END OF HEADER
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "END OF HEADER") {
		t.Error("missing END OF HEADER")
	}

	// Check required header labels
	requiredLabels := []string{
		"MARKER NUMBER",
		"OBSERVER / AGENCY",
		"REC # / TYPE / VERS",
		"ANT # / TYPE",
		"APPROX POSITION XYZ",
		"ANTENNA: DELTA H/E/N",
		"WAVELENGTH FACT L1/2",
		"INTERVAL",
		"TIME OF FIRST OBS",
		"TIME OF LAST OBS",
	}
	for _, label := range requiredLabels {
		found := false
		for _, l := range lines {
			if strings.Contains(l, label) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing header label: %s", label)
		}
	}

	// Check label is right-justified in cols 61-80
	for i, line := range lines {
		label := line[60:]
		trimmed := strings.TrimRight(label, " ")
		if trimmed == "" {
			continue
		}
		// Label should not start with leading spaces before content
		// (it's left-justified within the 20-char label field)
		if len(label) != 20 {
			t.Errorf("line %d label field not 20 chars: %q", i, label)
		}
	}
}

func TestWriteHeaderPositionFormat(t *testing.T) {
	var buf bytes.Buffer
	meta := testMetadata2()
	rw := NewWriter2(&buf, meta)
	if err := rw.WriteHeader(); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for _, line := range lines {
		if strings.Contains(line, "APPROX POSITION XYZ") {
			content := line[:60]
			// Should have 3 values each 14.4f
			if !strings.Contains(content, "-2694892.4600") {
				t.Errorf("X coordinate not formatted correctly: %q", content)
			}
			if !strings.Contains(content, "-4297439.0200") {
				t.Errorf("Y coordinate not formatted correctly: %q", content)
			}
			if !strings.Contains(content, "3854263.1100") {
				t.Errorf("Z coordinate not formatted correctly: %q", content)
			}
			break
		}
	}
}

func TestEpochLineFormatting(t *testing.T) {
	sats := []gnss.SatObs{
		makeSatObs(1, nil),
		makeSatObs(5, nil),
		makeSatObs(10, nil),
	}
	epoch := makeEpoch(2345, 259200_000_000_000, sats)
	line := formatEpochLine(epoch)

	parts := strings.Split(strings.TrimRight(line, "\n"), "\n")
	if len(parts) != 1 {
		t.Errorf("expected 1 line, got %d", len(parts))
	}

	// Check sat IDs
	if !strings.Contains(parts[0], "G01") {
		t.Error("missing G01")
	}
	if !strings.Contains(parts[0], "G05") {
		t.Error("missing G05")
	}
	if !strings.Contains(parts[0], "G10") {
		t.Error("missing G10")
	}

	// Check number of sats
	if !strings.Contains(parts[0], "  3") {
		t.Error("sat count should be 3")
	}

	// Check epoch flag 0
	if !strings.Contains(parts[0], "  0") {
		t.Error("epoch flag should be 0")
	}
}

func TestEpochLineContinuation(t *testing.T) {
	// 14 satellites → needs continuation line
	var sats []gnss.SatObs
	for i := uint8(1); i <= 14; i++ {
		sats = append(sats, makeSatObs(i, nil))
	}
	epoch := makeEpoch(2345, 259200_000_000_000, sats)
	line := formatEpochLine(epoch)

	parts := strings.Split(strings.TrimRight(line, "\n"), "\n")
	if len(parts) != 2 {
		t.Errorf("expected 2 lines for 14 sats, got %d", len(parts))
	}

	// Check continuation line starts with 32 spaces
	if len(parts) > 1 && !strings.HasPrefix(parts[1], strings.Repeat(" ", 32)) {
		t.Error("continuation line should start with 32 spaces")
	}

	// Check sats 13 and 14 are on second line
	if len(parts) > 1 {
		if !strings.Contains(parts[1], "G13") {
			t.Error("G13 should be on continuation line")
		}
		if !strings.Contains(parts[1], "G14") {
			t.Error("G14 should be on continuation line")
		}
	}
}

func TestObservationFormatting(t *testing.T) {
	signals := []gnss.Signal{
		{
			GnssID:       0,
			SigID:        0,
			FreqBand:     0, // L1
			Pseudorange:  22345678.123,
			CarrierPhase: 117456789.456,
			Doppler:      -1234.567,
			SNR:          42.0,
			LockTimeSec:  100,
			PRValid:      true,
			CPValid:      true,
		},
	}
	sat := makeSatObs(1, signals)
	rw := NewWriter2(nil, gnss.Metadata{})
	result := rw.formatObsLines(sat)

	// C1 should be present with F14.3
	if !strings.Contains(result, "22345678.123") {
		t.Error("C1 pseudorange not formatted correctly")
	}
	// L1 carrier phase
	if !strings.Contains(result, "117456789.456") {
		t.Error("L1 carrier phase not formatted correctly")
	}
	// D1 Doppler
	if !strings.Contains(result, "-1234.567") {
		t.Error("D1 Doppler not formatted correctly")
	}
	// S1 SNR
	if !strings.Contains(result, "42.000") {
		t.Error("S1 SNR not formatted correctly")
	}
}

func TestMissingObservationBlanks(t *testing.T) {
	// Only L1 signals, no L2 → C2, L2, D2, S2 should be blank (16 spaces each)
	signals := []gnss.Signal{
		{
			GnssID:       0,
			SigID:        0,
			FreqBand:     0,
			Pseudorange:  22345678.123,
			CarrierPhase: 117456789.456,
			Doppler:      -1234.567,
			SNR:          42.0,
			LockTimeSec:  100,
			PRValid:      true,
			CPValid:      true,
		},
	}
	sat := makeSatObs(1, signals)
	rw := NewWriter2(nil, gnss.Metadata{})
	result := rw.formatObsLines(sat)

	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	// With 8 obs types: 5 on first line, 3 on second
	if len(lines) != 2 {
		t.Fatalf("expected 2 data lines, got %d", len(lines))
	}

	// C2 is obs index 1, on first line after C1 (index 0)
	// Each obs is 16 chars wide. C2 starts at pos 16.
	firstLine := lines[0]
	c2Field := firstLine[16:32]
	if strings.TrimSpace(c2Field) != "" {
		t.Errorf("C2 should be blank for missing L2, got %q", c2Field)
	}
}

func TestSNRMapping(t *testing.T) {
	tests := []struct {
		snr  float64
		want int
	}{
		{5, 1},
		{12, 2},
		{18, 3},
		{24, 4},
		{30, 5},
		{36, 6},
		{42, 7},
		{48, 8},
		{54, 9},
		{60, 9},
	}
	for _, tt := range tests {
		got := snrToSS(tt.snr)
		if got != tt.want {
			t.Errorf("snrToSS(%v) = %d, want %d", tt.snr, got, tt.want)
		}
	}
}

func TestLLIFlagCycleSlip(t *testing.T) {
	signals := []gnss.Signal{
		{
			GnssID:       0,
			SigID:        0,
			FreqBand:     0,
			Pseudorange:  22345678.123,
			CarrierPhase: 117456789.456,
			Doppler:      -1234.567,
			SNR:          42.0,
			LockTimeSec:  0, // lock time reset → LLI=1
			PRValid:      true,
			CPValid:      true,
		},
	}
	sat := makeSatObs(1, signals)
	rw := NewWriter2(nil, gnss.Metadata{})
	result := rw.formatObsLines(sat)

	// L1 is obs index 2, starts at char 32 on first line, 14 digits + LLI flag
	firstLine := strings.Split(result, "\n")[0]
	// L1 field: positions 32..47 (14.3f = 14 chars, then LLI at 46, SS at 47)
	l1Field := firstLine[32:48]
	// LLI flag is at position 14 within the 16-char field
	lli := l1Field[14]
	if lli != '1' {
		t.Errorf("LLI flag for lock reset should be 1, got %c (field: %q)", lli, l1Field)
	}
}

func TestBestSignalSelection(t *testing.T) {
	signals := []gnss.Signal{
		{SigID: 4, FreqBand: 1, Pseudorange: 100, PRValid: true},
		{SigID: 3, FreqBand: 1, Pseudorange: 200, PRValid: true},
		{SigID: 0, FreqBand: 0, Pseudorange: 300, PRValid: true},
	}

	l1 := bestSignalForBand(signals, 0)
	if l1 == nil || l1.SigID != 0 {
		t.Error("L1 should select sigId=0")
	}

	l2 := bestSignalForBand(signals, 1)
	if l2 == nil || l2.SigID != 3 {
		t.Errorf("L2 should prefer sigId=3, got sigId=%d", l2.SigID)
	}
}

func TestEndToEnd(t *testing.T) {
	meta := testMetadata2()

	epoch1 := makeEpoch(2345, 259200_000_000_000, []gnss.SatObs{
		makeSatObs(1, []gnss.Signal{
			{FreqBand: 0, SigID: 0, Pseudorange: 22345678.123, CarrierPhase: 117456789.456,
				Doppler: -1234.567, SNR: 42.0, LockTimeSec: 100, PRValid: true, CPValid: true},
			{FreqBand: 1, SigID: 3, Pseudorange: 22345700.456, CarrierPhase: 91500000.789,
				Doppler: -960.123, SNR: 35.0, LockTimeSec: 50, PRValid: true, CPValid: true},
		}),
		makeSatObs(5, []gnss.Signal{
			{FreqBand: 0, SigID: 0, Pseudorange: 24000000.000, CarrierPhase: 126000000.000,
				Doppler: 500.000, SNR: 38.0, LockTimeSec: 200, PRValid: true, CPValid: true},
		}),
	})

	epoch2 := makeEpoch(2345, 259230_000_000_000, []gnss.SatObs{
		makeSatObs(1, []gnss.Signal{
			{FreqBand: 0, SigID: 0, Pseudorange: 22345679.100, CarrierPhase: 117456790.400,
				Doppler: -1234.500, SNR: 41.0, LockTimeSec: 130, PRValid: true, CPValid: true},
		}),
		makeSatObs(10, []gnss.Signal{
			{FreqBand: 0, SigID: 0, Pseudorange: 25000000.000, CarrierPhase: 131000000.000,
				Doppler: 100.000, SNR: 45.0, LockTimeSec: 300, PRValid: true, CPValid: true},
			{FreqBand: 1, SigID: 4, Pseudorange: 25000100.000, CarrierPhase: 102000000.000,
				Doppler: 78.000, SNR: 30.0, LockTimeSec: 100, PRValid: true, CPValid: true},
		}),
	})

	epoch3 := makeEpoch(2345, 259260_000_000_000, []gnss.SatObs{
		makeSatObs(1, []gnss.Signal{
			{FreqBand: 0, SigID: 0, Pseudorange: 22345680.200, CarrierPhase: 117456791.500,
				Doppler: -1234.400, SNR: 40.0, LockTimeSec: 160, PRValid: true, CPValid: true},
		}),
	})

	var buf bytes.Buffer
	err := WriteRinex2(&buf, meta, []gnss.Epoch{epoch1, epoch2, epoch3})
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Check we have reasonable number of lines
	if len(lines) < 20 {
		t.Errorf("output too short: %d lines", len(lines))
	}

	// Find END OF HEADER
	headerEnd := -1
	for i, line := range lines {
		if strings.Contains(line, "END OF HEADER") {
			headerEnd = i
			break
		}
	}
	if headerEnd < 0 {
		t.Fatal("END OF HEADER not found")
	}

	// All header lines should be 80 chars
	for i := 0; i <= headerEnd; i++ {
		if len(lines[i]) != 80 {
			t.Errorf("header line %d length = %d, want 80: %q", i, len(lines[i]), lines[i])
		}
	}

	// Check there are 3 epoch markers after header
	epochCount := 0
	for i := headerEnd + 1; i < len(lines); i++ {
		// Epoch lines start with " YY" pattern
		if len(lines[i]) > 0 && lines[i][0] == ' ' && len(lines[i]) >= 32 {
			// Check if this looks like an epoch line (has flag field)
			if strings.Contains(lines[i], "  0") && (strings.Contains(lines[i], "G0") || strings.Contains(lines[i], "G1")) {
				epochCount++
			}
		}
	}
	if epochCount != 3 {
		t.Errorf("expected 3 epochs, found %d", epochCount)
	}

	// Verify C2 in header, not P2
	if !strings.Contains(output, "C2") {
		t.Error("output should contain C2")
	}
	// P2 should not appear in the obs types line
	for _, line := range lines[:headerEnd] {
		if strings.Contains(line, "# / TYPES OF OBSERV") && strings.Contains(line, "P2") {
			t.Error("obs types should not contain P2")
		}
	}

	// Verify observation data contains expected values
	if !strings.Contains(output, "22345678.123") {
		t.Error("missing pseudorange value 22345678.123")
	}
	if !strings.Contains(output, "117456789.456") {
		t.Error("missing carrier phase value")
	}
}

func TestGnssTimeConversion(t *testing.T) {
	// GPS epoch: Jan 6, 1980. Week 0 TOW 0.
	gt := gnss.GNSSTime{Week: 0, TOWNanos: 0}
	y, mo, d, _, _, _ := GNSSTimeToCalendar(gt)
	if y != 1980 || mo != 1 || d != 6 {
		t.Errorf("GPS epoch should be 1980-01-06, got %d-%d-%d", y, mo, d)
	}

	// Week 2345 Wednesday 00:00:00
	gt2 := gnss.GNSSTime{Week: 2345, TOWNanos: 259200_000_000_000}
	y2, _, _, _, _, _ := GNSSTimeToCalendar(gt2)
	if y2 < 2020 || y2 > 2030 {
		t.Errorf("week 2345 should be around 2025, got year %d", y2)
	}
}

func TestNonGPSSatellitesFiltered(t *testing.T) {
	epoch := gnss.Epoch{
		Time: gnss.GNSSTime{Week: 2345, TOWNanos: 259200_000_000_000},
		Flag: 0,
		Satellites: []gnss.SatObs{
			{Constellation: gnss.ConsGPS, PRN: 1},
			{Constellation: gnss.ConsGLONASS, PRN: 5},
			{Constellation: gnss.ConsGalileo, PRN: 10},
		},
	}

	var buf bytes.Buffer
	meta := testMetadata2()
	rw := NewWriter2(&buf, meta)
	if err := rw.WriteEpoch(epoch); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if strings.Contains(output, "R05") {
		t.Error("GLONASS satellite should be filtered out")
	}
	if strings.Contains(output, "E10") {
		t.Error("Galileo satellite should be filtered out")
	}
	if !strings.Contains(output, "G01") {
		t.Error("GPS satellite should be present")
	}
}

func TestHalfCycleExcluded(t *testing.T) {
	signals := []gnss.Signal{
		{
			FreqBand:     0,
			SigID:        0,
			Pseudorange:  22345678.123,
			CarrierPhase: 117456789.456,
			Doppler:      -1234.567,
			SNR:          42.0,
			LockTimeSec:  100,
			PRValid:      true,
			CPValid:      true,
			HalfCycle:    true, // half-cycle ambiguity unresolved
		},
	}
	sat := makeSatObs(1, signals)
	rw := NewWriter2(nil, gnss.Metadata{})
	result := rw.formatObsLines(sat)

	// C1 should still be present (pseudorange unaffected by half-cycle)
	if !strings.Contains(result, "22345678.123") {
		t.Error("C1 should still be present even with half-cycle")
	}

	// L1 carrier phase should be PRESENT with LLI=2 (half-cycle not resolved)
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	firstLine := lines[0]
	l1Field := firstLine[32:48]
	if !strings.Contains(l1Field, "117456789.456") {
		t.Errorf("L1 carrier phase should be present with LLI flag, got %q", l1Field)
	}
	lli := l1Field[14]
	if lli != '2' {
		t.Errorf("LLI should be 2 (half-cycle unresolved), got %c (field: %q)", lli, l1Field)
	}
}

func TestHalfCycleSubtractedIncluded(t *testing.T) {
	// When SubHalfCyc=true, the half-cycle has been corrected — phase should be emitted
	signals := []gnss.Signal{
		{
			FreqBand:     0,
			SigID:        0,
			Pseudorange:  22345678.123,
			CarrierPhase: 117456789.456,
			Doppler:      -1234.567,
			SNR:          42.0,
			LockTimeSec:  100,
			PRValid:      true,
			CPValid:      true,
			HalfCycle:    true,
			SubHalfCyc:   true, // already corrected
		},
	}
	sat := makeSatObs(1, signals)
	rw := NewWriter2(nil, gnss.Metadata{})
	result := rw.formatObsLines(sat)

	if !strings.Contains(result, "117456789.456") {
		t.Error("L1 carrier phase should be present when SubHalfCyc=true")
	}
}

func TestLockTimeDecreaseLLI(t *testing.T) {
	// First epoch: lock time = 100s
	sig1 := []gnss.Signal{{
		FreqBand: 0, SigID: 0,
		Pseudorange: 22000000.0, CarrierPhase: 115000000.0, Doppler: -1234.5, SNR: 42.0,
		LockTimeSec: 100, PRValid: true, CPValid: true,
	}}
	sat1 := makeSatObs(1, sig1)

	rw := NewWriter2(nil, gnss.Metadata{})
	rw.formatObsLines(sat1)
	rw.updateArcs2(sat1)

	// Second epoch: lock time = 15s (decreased → cycle slip)
	sig2 := []gnss.Signal{{
		FreqBand: 0, SigID: 0,
		Pseudorange: 22000100.0, CarrierPhase: 115000500.0, Doppler: -1234.6, SNR: 43.0,
		LockTimeSec: 15, PRValid: true, CPValid: true,
	}}
	sat2 := makeSatObs(1, sig2)
	result := rw.formatObsLines(sat2)

	firstLine := strings.Split(result, "\n")[0]
	l1Field := firstLine[32:48]
	lli := l1Field[14]
	if lli != '1' {
		t.Errorf("LLI should be 1 when lock time decreases, got %c (field: %q)", lli, l1Field)
	}
}

func TestPhaseResumptionLLI(t *testing.T) {
	rw := NewWriter2(nil, gnss.Metadata{})

	// Epoch 1: phase present, no half-cycle
	sig1 := []gnss.Signal{{
		FreqBand: 0, SigID: 0,
		Pseudorange: 22000000.0, CarrierPhase: 115000000.0, Doppler: -1234.5, SNR: 42.0,
		LockTimeSec: 100, PRValid: true, CPValid: true,
	}}
	sat1 := makeSatObs(1, sig1)
	rw.formatObsLines(sat1)
	rw.updateArcs2(sat1)

	// Epoch 2: phase absent (CPValid=false, satellite tracking lost)
	sig2 := []gnss.Signal{{
		FreqBand: 0, SigID: 0,
		Pseudorange: 22000100.0, CarrierPhase: 0, Doppler: 0, SNR: 20.0,
		LockTimeSec: 0, PRValid: true, CPValid: false,
	}}
	sat2 := makeSatObs(1, sig2)
	rw.formatObsLines(sat2)
	rw.updateArcs2(sat2)

	// Epoch 3: phase resumes — should have LLI=1
	sig3 := []gnss.Signal{{
		FreqBand: 0, SigID: 0,
		Pseudorange: 22000200.0, CarrierPhase: 115001000.0, Doppler: -1234.7, SNR: 41.0,
		LockTimeSec: 5, PRValid: true, CPValid: true,
	}}
	sat3 := makeSatObs(1, sig3)
	result := rw.formatObsLines(sat3)

	firstLine := strings.Split(result, "\n")[0]
	l1Field := firstLine[32:48]
	lli := l1Field[14]
	if lli != '1' {
		t.Errorf("LLI should be 1 when phase resumes after gap, got %c (field: %q)", lli, l1Field)
	}
}

func TestHalfCycleTransitionLLI(t *testing.T) {
	rw := NewWriter2(nil, gnss.Metadata{})

	// Epoch 1: SubHalfCyc=false
	sig1 := []gnss.Signal{{
		FreqBand: 0, SigID: 0,
		Pseudorange: 22000000.0, CarrierPhase: 115000000.0, Doppler: -1234.5, SNR: 42.0,
		LockTimeSec: 100, PRValid: true, CPValid: true,
		SubHalfCyc: false,
	}}
	sat1 := makeSatObs(1, sig1)
	rw.formatObsLines(sat1)
	rw.updateArcs2(sat1)

	// Epoch 2: SubHalfCyc transitions to true → should trigger LLI=1 (slip)
	sig2 := []gnss.Signal{{
		FreqBand: 0, SigID: 0,
		Pseudorange: 22000100.0, CarrierPhase: 115000500.0, Doppler: -1234.6, SNR: 43.0,
		LockTimeSec: 130, PRValid: true, CPValid: true,
		SubHalfCyc: true,
	}}
	sat2 := makeSatObs(1, sig2)
	result := rw.formatObsLines(sat2)

	firstLine := strings.Split(result, "\n")[0]
	l1Field := firstLine[32:48]
	lli := l1Field[14]
	if lli != '1' {
		t.Errorf("LLI should be 1 when SubHalfCyc transitions, got %c (field: %q)", lli, l1Field)
	}
}
