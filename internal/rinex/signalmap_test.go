package rinex

import (
	"testing"

	"github.com/jakevis/rinexprep/internal/gnss"
)

func TestAllGPSMappingsPresent(t *testing.T) {
	gpsSigIDs := []uint8{0, 3, 4, 5, 6}
	for _, sid := range gpsSigIDs {
		if _, ok := LookupMapping(0, sid); !ok {
			t.Errorf("GPS mapping missing for sigId=%d", sid)
		}
	}
}

func TestLookupMappingKnown(t *testing.T) {
	tests := []struct {
		gnssID uint8
		sigID  uint8
		desc   string
		r3pr   string
	}{
		{0, 0, "GPS L1 C/A", "C1C"},
		{0, 3, "GPS L2 CL", "C2L"},
		{6, 0, "GLONASS L1 C/A", "C1C"},
		{6, 2, "GLONASS L2 C/A", "C2C"},
		{2, 0, "Galileo E1 C", "C1C"},
		{2, 5, "Galileo E5b I", "C7I"},
		{3, 0, "BeiDou B1I D1", "C2I"},
		{3, 2, "BeiDou B2I D1", "C7I"},
	}
	for _, tt := range tests {
		m, ok := LookupMapping(tt.gnssID, tt.sigID)
		if !ok {
			t.Errorf("LookupMapping(%d,%d) not found; want %s", tt.gnssID, tt.sigID, tt.desc)
			continue
		}
		if m.Rinex3.Pseudorange != tt.r3pr {
			t.Errorf("LookupMapping(%d,%d) Rinex3.Pseudorange = %q; want %q",
				tt.gnssID, tt.sigID, m.Rinex3.Pseudorange, tt.r3pr)
		}
		if m.Desc != tt.desc {
			t.Errorf("LookupMapping(%d,%d) Desc = %q; want %q",
				tt.gnssID, tt.sigID, m.Desc, tt.desc)
		}
	}
}

func TestLookupMappingUnknown(t *testing.T) {
	unknowns := [][2]uint8{{0, 99}, {7, 0}, {255, 255}}
	for _, u := range unknowns {
		if _, ok := LookupMapping(u[0], u[1]); ok {
			t.Errorf("LookupMapping(%d,%d) should return false for unknown pair", u[0], u[1])
		}
	}
}

func TestBestSignalPerBandPreference(t *testing.T) {
	// sigId=3 (priority 1) should beat sigId=4 (priority 2) on GPS L2.
	signals := []gnss.Signal{
		{GnssID: 0, SigID: 4, FreqBand: 1, Pseudorange: 22000000, PRValid: true},
		{GnssID: 0, SigID: 3, FreqBand: 1, Pseudorange: 21000000, PRValid: true},
		{GnssID: 0, SigID: 0, FreqBand: 0, Pseudorange: 20000000, PRValid: true},
	}
	best := BestSignalPerBand(signals)

	// L1 band: only sigId=0
	if s, ok := best[0]; !ok || s.SigID != 0 {
		t.Error("expected sigId=0 as best L1 signal")
	}
	// L2 band: sigId=3 preferred over sigId=4
	if s, ok := best[1]; !ok || s.SigID != 3 {
		t.Errorf("expected sigId=3 as best L2 signal; got sigId=%d", best[1].SigID)
	}
}

func TestBestSignalPerBandSkipsUnknown(t *testing.T) {
	signals := []gnss.Signal{
		{GnssID: 0, SigID: 99}, // unknown mapping
	}
	best := BestSignalPerBand(signals)
	if len(best) != 0 {
		t.Errorf("expected empty map for unknown signals; got %d entries", len(best))
	}
}

func TestC2NeverP2(t *testing.T) {
	// All GPS L2 mappings must use C2, not P2.
	l2SigIDs := []uint8{3, 4}
	for _, sid := range l2SigIDs {
		m, ok := LookupMapping(0, sid)
		if !ok {
			t.Fatalf("GPS sigId=%d not found", sid)
		}
		if m.Rinex2.Pseudorange != "C2" {
			t.Errorf("GPS sigId=%d Rinex2.Pseudorange = %q; want \"C2\" (never P2)",
				sid, m.Rinex2.Pseudorange)
		}
	}
}

func TestRinex2ObsTypes(t *testing.T) {
	want := []string{"C1", "C2", "L1", "L2", "D1", "D2", "S1", "S2"}
	got := Rinex2ObsTypes()
	if len(got) != len(want) {
		t.Fatalf("Rinex2ObsTypes() len = %d; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Rinex2ObsTypes()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestRinex3ObsTypesGPS(t *testing.T) {
	codes := Rinex3ObsTypes(0)
	if len(codes) == 0 {
		t.Fatal("Rinex3ObsTypes(0) returned empty")
	}
	// Expect sorted; check a few known codes are present.
	wantCodes := map[string]bool{
		"C1C": true, "L1C": true, "D1C": true, "S1C": true,
		"C2L": true, "L2L": true, "D2L": true, "S2L": true,
		"C2S": true, "L2S": true,
	}
	for _, c := range codes {
		delete(wantCodes, c)
	}
	for c := range wantCodes {
		t.Errorf("Rinex3ObsTypes(0) missing expected code %q", c)
	}
	// Verify sorted order.
	for i := 1; i < len(codes); i++ {
		if codes[i] < codes[i-1] {
			t.Errorf("Rinex3ObsTypes(0) not sorted: %q before %q", codes[i-1], codes[i])
		}
	}
}

func TestRinex3ObsTypesUnknown(t *testing.T) {
	codes := Rinex3ObsTypes(99)
	if len(codes) != 0 {
		t.Errorf("Rinex3ObsTypes(99) should be empty; got %v", codes)
	}
}
