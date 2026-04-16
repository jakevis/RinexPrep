package gnss

import "fmt"

// Constellation identifies a GNSS satellite system.
type Constellation uint8

const (
	ConsGPS     Constellation = 0 // GPS (US)
	ConsSBAS    Constellation = 1 // SBAS
	ConsGalileo Constellation = 2 // Galileo (EU)
	ConsBeiDou  Constellation = 3 // BeiDou (China)
	ConsIMES    Constellation = 4 // IMES (Japan)
	ConsQZSS   Constellation = 5 // QZSS (Japan)
	ConsGLONASS Constellation = 6 // GLONASS (Russia)
)

// String returns the single-character RINEX 3 system identifier.
func (c Constellation) String() string {
	switch c {
	case ConsGPS:
		return "G"
	case ConsGLONASS:
		return "R"
	case ConsGalileo:
		return "E"
	case ConsBeiDou:
		return "C"
	case ConsSBAS:
		return "S"
	case ConsQZSS:
		return "J"
	default:
		return "?"
	}
}

// Signal represents a single GNSS signal measurement from a receiver.
// This is the raw observation before any RINEX code mapping.
type Signal struct {
	// GnssID is the UBX gnssId (matches Constellation values).
	GnssID uint8
	// SigID is the UBX sigId within the constellation.
	SigID uint8

	// FreqBand identifies the frequency band (L1=0, L2=1, L5=2, etc.)
	FreqBand uint8

	// Raw observables
	Pseudorange  float64 // meters
	CarrierPhase float64 // cycles
	Doppler      float64 // Hz
	SNR          float64 // dB-Hz

	// Quality indicators
	LockTimeSec float64 // continuous lock time in seconds
	PRValid     bool    // pseudorange valid
	CPValid     bool    // carrier phase valid
	HalfCycle   bool    // half-cycle ambiguity (not yet resolved)
	SubHalfCyc  bool    // half-cycle subtracted
}

// SatObs groups all signals for one satellite at one epoch.
type SatObs struct {
	Constellation Constellation
	PRN           uint8
	Signals       []Signal
}

// SatID returns a unique string identifier like "G01", "R24".
func (s SatObs) SatID() string {
	return s.Constellation.String() + fmt.Sprintf("%02d", s.PRN)
}

// Epoch represents one measurement epoch with all observed satellites.
type Epoch struct {
	Time       GNSSTime
	Satellites []SatObs

	// EpochFlag per RINEX spec: 0=OK, 1=power failure, >1=event
	Flag uint8
}

// GPSSatCount returns the number of GPS satellites with at least one valid signal.
func (e Epoch) GPSSatCount() int {
	count := 0
	for _, sat := range e.Satellites {
		if sat.Constellation == ConsGPS {
			count++
		}
	}
	return count
}

// FilterGPSOnly returns a new Epoch containing only GPS satellites.
func (e Epoch) FilterGPSOnly() Epoch {
	filtered := Epoch{
		Time: e.Time,
		Flag: e.Flag,
	}
	for _, sat := range e.Satellites {
		if sat.Constellation == ConsGPS {
			filtered.Satellites = append(filtered.Satellites, sat)
		}
	}
	return filtered
}
