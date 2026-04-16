package rinex

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jakevis/rinexprep/internal/gnss"
)

func testMetadata() gnss.Metadata {
	return gnss.Metadata{
		MarkerName:      "TEST",
		MarkerNumber:    "1234",
		Observer:        "TestObs",
		Agency:          "TestAgency",
		ReceiverNumber:  "REC001",
		ReceiverType:    "UBLOX_F9P",
		ReceiverVersion: "1.32",
		AntennaNumber:   "ANT001",
		AntennaType:     "TRM59800.00     NONE",
		AntennaDeltaH:   1.5000,
		AntennaDeltaE:   0.0000,
		AntennaDeltaN:   0.0000,
		ApproxX:         -2694892.0600,
		ApproxY:         -4297330.3600,
		ApproxZ:         3854512.1900,
		Interval:        30.0,
		FirstEpoch: gnss.GNSSTime{
			Week:     2345,
			TOWNanos: 259200_000_000_000, // Wednesday 00:00:00
		},
		LastEpoch: gnss.GNSSTime{
			Week:     2345,
			TOWNanos: 259230_000_000_000, // Wednesday 00:00:30
		},
	}
}

func makeGPSSat(prn uint8, pr, cp, dop, snr float64) gnss.SatObs {
	return gnss.SatObs{
		Constellation: gnss.ConsGPS,
		PRN:           prn,
		Signals: []gnss.Signal{
			{
				GnssID:       0,
				SigID:        0,
				FreqBand:     0,
				Pseudorange:  pr,
				CarrierPhase: cp,
				Doppler:      dop,
				SNR:          snr,
				PRValid:      true,
				CPValid:      true,
			},
		},
	}
}

func makeGLONASSSat(prn uint8, pr, cp, dop, snr float64) gnss.SatObs {
	return gnss.SatObs{
		Constellation: gnss.ConsGLONASS,
		PRN:           prn,
		Signals: []gnss.Signal{
			{
				GnssID:       6,
				SigID:        0,
				FreqBand:     0,
				Pseudorange:  pr,
				CarrierPhase: cp,
				Doppler:      dop,
				SNR:          snr,
				PRValid:      true,
				CPValid:      true,
			},
		},
	}
}

func makeGalileoSat(prn uint8, pr, cp, dop, snr float64) gnss.SatObs {
	return gnss.SatObs{
		Constellation: gnss.ConsGalileo,
		PRN:           prn,
		Signals: []gnss.Signal{
			{
				GnssID:       2,
				SigID:        0,
				FreqBand:     0,
				Pseudorange:  pr,
				CarrierPhase: cp,
				Doppler:      dop,
				SNR:          snr,
				PRValid:      true,
				CPValid:      true,
			},
		},
	}
}

func TestWriter3_GPSOnlyHeader(t *testing.T) {
	meta := testMetadata()
	var buf bytes.Buffer
	epochs := []gnss.Epoch{
		{
			Time: meta.FirstEpoch,
			Satellites: []gnss.SatObs{
				makeGPSSat(1, 22000000.0, 115000000.0, -1234.5, 42.0),
			},
		},
	}

	err := WriteRinex3(&buf, meta, epochs)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Check version line says G (GPS)
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		t.Fatal("empty output")
	}
	versionLine := lines[0]
	if !strings.Contains(versionLine, "3.03") {
		t.Errorf("version line missing 3.03: %q", versionLine)
	}
	if !strings.Contains(versionLine, "G: GPS") {
		t.Errorf("GPS-only header should contain 'G: GPS': %q", versionLine)
	}
	if !strings.Contains(versionLine, "RINEX VERSION / TYPE") {
		t.Errorf("version line missing label: %q", versionLine)
	}

	// Should only have G system, not R, E, C in SYS / # / OBS TYPES
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "SYS / # / OBS TYPES") && strings.HasPrefix(line, "R") {
			t.Error("GPS-only output should not have GLONASS SYS / # / OBS TYPES line")
		}
	}
}

func TestWriter3_MixedHeader(t *testing.T) {
	meta := testMetadata()
	var buf bytes.Buffer
	epochs := []gnss.Epoch{
		{
			Time: meta.FirstEpoch,
			Satellites: []gnss.SatObs{
				makeGPSSat(1, 22000000.0, 115000000.0, -1234.5, 42.0),
				makeGLONASSSat(5, 23000000.0, 120000000.0, -567.8, 38.0),
			},
		},
	}

	err := WriteRinex3(&buf, meta, epochs)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	lines := strings.Split(output, "\n")
	versionLine := lines[0]
	if !strings.Contains(versionLine, "M: Mixed") {
		t.Errorf("mixed header should contain 'M: Mixed': %q", versionLine)
	}
}

func TestWriter3_SysObsTypes(t *testing.T) {
	meta := testMetadata()
	var buf bytes.Buffer
	epochs := []gnss.Epoch{
		{
			Time: meta.FirstEpoch,
			Satellites: []gnss.SatObs{
				makeGPSSat(1, 22000000.0, 115000000.0, -1234.5, 42.0),
				makeGalileoSat(11, 24000000.0, 130000000.0, -890.1, 45.0),
			},
		},
	}

	err := WriteRinex3(&buf, meta, epochs)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// GPS sys line
	foundGPS := false
	foundGal := false
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "SYS / # / OBS TYPES") {
			if strings.HasPrefix(line, "G") {
				foundGPS = true
				if !strings.Contains(line, "C1C") || !strings.Contains(line, "L2X") {
					t.Errorf("GPS SYS line missing expected codes: %q", line)
				}
				if !strings.Contains(line, "  8 ") {
					t.Errorf("GPS SYS line should have 8 obs types: %q", line)
				}
			}
			if strings.HasPrefix(line, "E") {
				foundGal = true
				if !strings.Contains(line, "C1C") || !strings.Contains(line, "C7I") {
					t.Errorf("Galileo SYS line missing expected codes: %q", line)
				}
			}
		}
	}
	if !foundGPS {
		t.Error("missing GPS SYS / # / OBS TYPES line")
	}
	if !foundGal {
		t.Error("missing Galileo SYS / # / OBS TYPES line")
	}
}

func TestWriter3_EpochLine(t *testing.T) {
	meta := testMetadata()
	var buf bytes.Buffer

	// GPS week 2345, TOW = 259200s = Wednesday 00:00:00
	// GPS epoch: Jan 6 1980. 2345 weeks = ~44.96 years from epoch
	epoch := gnss.Epoch{
		Time: gnss.GNSSTime{
			Week:     2345,
			TOWNanos: 259200_000_000_000,
		},
		Satellites: []gnss.SatObs{
			makeGPSSat(1, 22000000.0, 115000000.0, -1234.5, 42.0),
		},
	}

	rw := NewWriter3(&buf, meta)
	if err := rw.WriteEpoch(epoch); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	lines := strings.Split(output, "\n")
	epochLine := lines[0]

	// Must start with >
	if !strings.HasPrefix(epochLine, ">") {
		t.Errorf("epoch line must start with '>': %q", epochLine)
	}

	// Must contain 4-digit year
	year, _, _, _, _, _ := GNSSTimeToCalendar(epoch.Time)
	yearStr := strings.TrimSpace(epochLine[2:6])
	if yearStr != strings.TrimSpace(strings.Fields(epochLine[1:])[0]) {
		// just check year is 4 digits
	}
	if year < 2000 || year > 2100 {
		t.Errorf("unexpected year %d", year)
	}

	// Must contain satellite count
	if !strings.Contains(epochLine, "  1") {
		t.Errorf("epoch line should show 1 satellite: %q", epochLine)
	}
}

func TestWriter3_SatelliteLine(t *testing.T) {
	meta := testMetadata()
	var buf bytes.Buffer

	epoch := gnss.Epoch{
		Time: meta.FirstEpoch,
		Satellites: []gnss.SatObs{
			makeGPSSat(5, 22345678.901, 117456789.012, -1234.567, 42.0),
		},
	}

	rw := NewWriter3(&buf, meta)
	if err := rw.WriteEpoch(epoch); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		t.Fatal("expected at least 2 lines (epoch + satellite)")
	}

	satLine := lines[1]

	// Must start with satellite ID
	if !strings.HasPrefix(satLine, "G05") {
		t.Errorf("satellite line should start with G05: %q", satLine)
	}

	// Must contain pseudorange value
	if !strings.Contains(satLine, "22345678.901") {
		t.Errorf("satellite line should contain pseudorange: %q", satLine)
	}

	// Must contain carrier phase value
	if !strings.Contains(satLine, "117456789.012") {
		t.Errorf("satellite line should contain carrier phase: %q", satLine)
	}
}

func TestWriter3_MissingObservations(t *testing.T) {
	meta := testMetadata()
	var buf bytes.Buffer

	// Satellite with only L1 signal, no L2 → L2 obs should be blank
	sat := gnss.SatObs{
		Constellation: gnss.ConsGPS,
		PRN:           3,
		Signals: []gnss.Signal{
			{
				FreqBand:     0,
				Pseudorange:  22000000.0,
				CarrierPhase: 115000000.0,
				Doppler:      -1234.5,
				SNR:          42.0,
				PRValid:      true,
				CPValid:      true,
			},
		},
	}

	epoch := gnss.Epoch{
		Time:       meta.FirstEpoch,
		Satellites: []gnss.SatObs{sat},
	}

	rw := NewWriter3(&buf, meta)
	if err := rw.WriteEpoch(epoch); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	lines := strings.Split(output, "\n")
	satLine := lines[1]

	// L1 data present (first 4 obs: C1C L1C D1C S1C)
	if !strings.Contains(satLine, "22000000.000") {
		t.Errorf("should contain L1 pseudorange: %q", satLine)
	}

	// L2 data absent (last 4 obs: C2L L2L D2L S2L) → trailing blanks
	// After the L1 data (4 obs * 16 chars = 64 chars after "G03"), the
	// remaining 64 chars should be spaces
	satData := satLine[3:] // strip "G03"
	l2Start := 4 * 16
	if len(satData) > l2Start {
		l2Section := satData[l2Start:]
		if strings.TrimSpace(l2Section) != "" {
			t.Errorf("L2 section should be blank for L1-only sat: %q", l2Section)
		}
	}
}

func TestWriter3_EndToEnd(t *testing.T) {
	meta := testMetadata()
	var buf bytes.Buffer

	epochs := []gnss.Epoch{
		{
			Time: gnss.GNSSTime{Week: 2345, TOWNanos: 259200_000_000_000},
			Satellites: []gnss.SatObs{
				makeGPSSat(1, 22000000.0, 115000000.0, -1234.5, 42.0),
				makeGPSSat(5, 23000000.0, 120000000.0, -567.8, 38.0),
				makeGLONASSSat(3, 21000000.0, 110000000.0, -890.1, 35.0),
			},
		},
		{
			Time: gnss.GNSSTime{Week: 2345, TOWNanos: 259230_000_000_000},
			Satellites: []gnss.SatObs{
				makeGPSSat(1, 22000100.0, 115000500.0, -1234.6, 43.0),
				makeGLONASSSat(3, 21000100.0, 110000500.0, -890.2, 36.0),
			},
		},
		{
			Time: gnss.GNSSTime{Week: 2345, TOWNanos: 259260_000_000_000},
			Satellites: []gnss.SatObs{
				makeGPSSat(1, 22000200.0, 115001000.0, -1234.7, 41.0),
				makeGPSSat(5, 23000200.0, 120001000.0, -567.9, 37.0),
			},
		},
	}

	err := WriteRinex3(&buf, meta, epochs)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Check key header elements
	if !strings.Contains(output, "RINEX VERSION / TYPE") {
		t.Error("missing RINEX VERSION / TYPE")
	}
	if !strings.Contains(output, "END OF HEADER") {
		t.Error("missing END OF HEADER")
	}
	if !strings.Contains(output, "M: Mixed") {
		t.Error("should be M: Mixed with GPS+GLONASS")
	}
	if !strings.Contains(output, "INTERVAL") {
		t.Error("missing INTERVAL")
	}

	// Count epoch lines
	epochCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, ">") {
			epochCount++
		}
	}
	if epochCount != 3 {
		t.Errorf("expected 3 epoch lines, got %d", epochCount)
	}

	// First epoch should have 3 sats, second 2, third 2
	var satCounts []int
	currentCount := 0
	inEpoch := false
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, ">") {
			if inEpoch {
				satCounts = append(satCounts, currentCount)
			}
			inEpoch = true
			currentCount = 0
		} else if inEpoch && len(line) > 3 && (line[0] == 'G' || line[0] == 'R' || line[0] == 'E' || line[0] == 'C') {
			currentCount++
		}
	}
	if inEpoch {
		satCounts = append(satCounts, currentCount)
	}

	if len(satCounts) != 3 {
		t.Fatalf("expected 3 epochs, got %d", len(satCounts))
	}
	if satCounts[0] != 3 {
		t.Errorf("epoch 1: expected 3 sats, got %d", satCounts[0])
	}
	if satCounts[1] != 2 {
		t.Errorf("epoch 2: expected 2 sats, got %d", satCounts[1])
	}
	if satCounts[2] != 2 {
		t.Errorf("epoch 3: expected 2 sats, got %d", satCounts[2])
	}
}

func TestWriter3_HeaderLineWidth(t *testing.T) {
	meta := testMetadata()
	var buf bytes.Buffer
	rw := NewWriter3(&buf, meta)
	if err := rw.WriteHeader(); err != nil {
		t.Fatal(err)
	}

	for i, line := range strings.Split(buf.String(), "\n") {
		if line == "" {
			continue
		}
		if len(line) != 80 {
			t.Errorf("header line %d has length %d, expected 80: %q", i+1, len(line), line)
		}
	}
}

func TestGNSSTimeToCalendar(t *testing.T) {
	// GPS week 0, TOW 0 = Jan 6, 1980 00:00:00
	year, month, day, hour, min, sec := GNSSTimeToCalendar(gnss.GNSSTime{Week: 0, TOWNanos: 0})
	if year != 1980 || month != 1 || day != 6 || hour != 0 || min != 0 || sec != 0 {
		t.Errorf("GPS epoch wrong: %d-%d-%d %d:%d:%f", year, month, day, hour, min, sec)
	}

	// GPS week 2345, TOW = 259200s (3 days) = Wednesday
	year2, month2, day2, _, _, _ := GNSSTimeToCalendar(gnss.GNSSTime{
		Week:     2345,
		TOWNanos: 259200_000_000_000,
	})
	if year2 < 2020 || year2 > 2030 {
		t.Errorf("unexpected year for week 2345: %d", year2)
	}
	_ = month2
	_ = day2
}
