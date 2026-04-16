package gnss

import "testing"

// ---------------------------------------------------------------------------
// Constellation.String()
// ---------------------------------------------------------------------------

func TestConstellationString(t *testing.T) {
	tests := []struct {
		c    Constellation
		want string
	}{
		{ConsGPS, "G"},
		{ConsGLONASS, "R"},
		{ConsGalileo, "E"},
		{ConsBeiDou, "C"},
		{ConsSBAS, "S"},
		{ConsQZSS, "J"},
		{ConsIMES, "?"},
		{Constellation(255), "?"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.c.String(); got != tt.want {
				t.Errorf("Constellation(%d).String() = %q, want %q", tt.c, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SatObs.SatID()
// ---------------------------------------------------------------------------

func TestSatID(t *testing.T) {
	tests := []struct {
		c    Constellation
		prn  uint8
		want string
	}{
		{ConsGPS, 1, "G01"},
		{ConsGPS, 32, "G32"},
		{ConsGLONASS, 24, "R24"},
		{ConsGalileo, 5, "E05"},
		{ConsBeiDou, 14, "C14"},
		{ConsSBAS, 3, "S03"},
		{ConsQZSS, 7, "J07"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			s := SatObs{Constellation: tt.c, PRN: tt.prn}
			if got := s.SatID(); got != tt.want {
				t.Errorf("SatID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Epoch.GPSSatCount()
// ---------------------------------------------------------------------------

func TestGPSSatCount(t *testing.T) {
	e := Epoch{
		Satellites: []SatObs{
			{Constellation: ConsGPS, PRN: 1},
			{Constellation: ConsGPS, PRN: 5},
			{Constellation: ConsGLONASS, PRN: 3},
			{Constellation: ConsGalileo, PRN: 12},
			{Constellation: ConsGPS, PRN: 31},
		},
	}
	if got := e.GPSSatCount(); got != 3 {
		t.Errorf("GPSSatCount() = %d, want 3", got)
	}
}

func TestGPSSatCount_NoGPS(t *testing.T) {
	e := Epoch{
		Satellites: []SatObs{
			{Constellation: ConsGLONASS, PRN: 1},
			{Constellation: ConsGalileo, PRN: 2},
		},
	}
	if got := e.GPSSatCount(); got != 0 {
		t.Errorf("GPSSatCount() = %d, want 0", got)
	}
}

func TestGPSSatCount_Empty(t *testing.T) {
	e := Epoch{}
	if got := e.GPSSatCount(); got != 0 {
		t.Errorf("GPSSatCount() = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Epoch.FilterGPSOnly()
// ---------------------------------------------------------------------------

func TestFilterGPSOnly(t *testing.T) {
	original := Epoch{
		Time: GNSSTime{Week: 100, TOWNanos: 30e9},
		Flag: 1,
		Satellites: []SatObs{
			{Constellation: ConsGPS, PRN: 1},
			{Constellation: ConsGLONASS, PRN: 3},
			{Constellation: ConsGPS, PRN: 5},
			{Constellation: ConsGalileo, PRN: 12},
			{Constellation: ConsBeiDou, PRN: 7},
		},
	}
	filtered := original.FilterGPSOnly()

	if filtered.Time.Week != 100 || filtered.Time.TOWNanos != int64(30e9) {
		t.Errorf("FilterGPSOnly changed Time")
	}
	if filtered.Flag != 1 {
		t.Errorf("FilterGPSOnly changed Flag: got %d, want 1", filtered.Flag)
	}
	if len(filtered.Satellites) != 2 {
		t.Fatalf("FilterGPSOnly: got %d sats, want 2", len(filtered.Satellites))
	}
	for _, sat := range filtered.Satellites {
		if sat.Constellation != ConsGPS {
			t.Errorf("FilterGPSOnly left non-GPS sat: %s", sat.SatID())
		}
	}
}

func TestFilterGPSOnly_NoGPS(t *testing.T) {
	e := Epoch{
		Satellites: []SatObs{
			{Constellation: ConsGLONASS, PRN: 1},
		},
	}
	filtered := e.FilterGPSOnly()
	if len(filtered.Satellites) != 0 {
		t.Errorf("FilterGPSOnly: got %d sats, want 0", len(filtered.Satellites))
	}
}

func TestFilterGPSOnly_DoesNotMutateOriginal(t *testing.T) {
	original := Epoch{
		Satellites: []SatObs{
			{Constellation: ConsGPS, PRN: 1},
			{Constellation: ConsGLONASS, PRN: 3},
		},
	}
	_ = original.FilterGPSOnly()
	if len(original.Satellites) != 2 {
		t.Errorf("FilterGPSOnly mutated original: got %d sats, want 2", len(original.Satellites))
	}
}
