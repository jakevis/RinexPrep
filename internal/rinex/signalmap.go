package rinex

import (
	"sort"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// SignalMapping describes how a UBX (gnssId, sigId) pair maps to RINEX
// observation codes for both RINEX 2.11 and 3.x.
type SignalMapping struct {
	GnssID   uint8
	SigID    uint8
	FreqBand uint8 // 0=L1, 1=L2, 2=L5
	Rinex2   Rinex2Codes
	Rinex3   Rinex3Codes
	Priority int    // lower = preferred when multiple signals on same freq
	Desc     string // human-readable description
}

// Rinex2Codes holds the RINEX 2.11 observation type strings.
type Rinex2Codes struct {
	Pseudorange string // C1, C2, P1, P2
	Phase       string // L1, L2
	Doppler     string // D1, D2
	SNR         string // S1, S2
}

// Rinex3Codes holds the RINEX 3.x observation type strings.
type Rinex3Codes struct {
	Pseudorange string // C1C, C2L, etc.
	Phase       string // L1C, L2L, etc.
	Doppler     string // D1C, D2L, etc.
	SNR         string // S1C, S2L, etc.
}

// signalKey is used to index the mapping table.
type signalKey struct {
	gnssID uint8
	sigID  uint8
}

// signalTable is the complete mapping table keyed by (gnssId, sigId).
var signalTable = map[signalKey]SignalMapping{
	// ── GPS (gnssId=0) ──────────────────────────────────────────────
	{0, 0}: {
		GnssID: 0, SigID: 0, FreqBand: 0,
		Rinex2:   Rinex2Codes{"C1", "L1", "D1", "S1"},
		Rinex3:   Rinex3Codes{"C1C", "L1C", "D1C", "S1C"},
		Priority: 1, Desc: "GPS L1 C/A",
	},
	{0, 3}: {
		GnssID: 0, SigID: 3, FreqBand: 1,
		Rinex2:   Rinex2Codes{"C2", "L2", "D2", "S2"},
		Rinex3:   Rinex3Codes{"C2L", "L2L", "D2L", "S2L"},
		Priority: 1, Desc: "GPS L2 CL",
	},
	{0, 4}: {
		GnssID: 0, SigID: 4, FreqBand: 1,
		Rinex2:   Rinex2Codes{"C2", "L2", "D2", "S2"},
		Rinex3:   Rinex3Codes{"C2S", "L2S", "D2S", "S2S"},
		Priority: 2, Desc: "GPS L2 CM",
	},
	{0, 6}: {
		GnssID: 0, SigID: 6, FreqBand: 0,
		Rinex2:   Rinex2Codes{"C1", "L1", "D1", "S1"},
		Rinex3:   Rinex3Codes{"C1L", "L1L", "D1L", "S1L"},
		Priority: 2, Desc: "GPS L1 L1C (D)",
	},
	{0, 5}: {
		GnssID: 0, SigID: 5, FreqBand: 0,
		Rinex2:   Rinex2Codes{"C1", "L1", "D1", "S1"},
		Rinex3:   Rinex3Codes{"C1X", "L1X", "D1X", "S1X"},
		Priority: 3, Desc: "GPS L1 L1C (P)",
	},

	// ── GLONASS (gnssId=6) ──────────────────────────────────────────
	{6, 0}: {
		GnssID: 6, SigID: 0, FreqBand: 0,
		Rinex2:   Rinex2Codes{"C1", "L1", "D1", "S1"},
		Rinex3:   Rinex3Codes{"C1C", "L1C", "D1C", "S1C"},
		Priority: 1, Desc: "GLONASS L1 C/A",
	},
	{6, 2}: {
		GnssID: 6, SigID: 2, FreqBand: 1,
		Rinex2:   Rinex2Codes{"C2", "L2", "D2", "S2"},
		Rinex3:   Rinex3Codes{"C2C", "L2C", "D2C", "S2C"},
		Priority: 1, Desc: "GLONASS L2 C/A",
	},

	// ── Galileo (gnssId=2) ──────────────────────────────────────────
	{2, 0}: {
		GnssID: 2, SigID: 0, FreqBand: 0,
		Rinex3:   Rinex3Codes{"C1C", "L1C", "D1C", "S1C"},
		Priority: 1, Desc: "Galileo E1 C",
	},
	{2, 1}: {
		GnssID: 2, SigID: 1, FreqBand: 0,
		Rinex3:   Rinex3Codes{"C1B", "L1B", "D1B", "S1B"},
		Priority: 2, Desc: "Galileo E1 B",
	},
	{2, 5}: {
		GnssID: 2, SigID: 5, FreqBand: 2,
		Rinex3:   Rinex3Codes{"C7I", "L7I", "D7I", "S7I"},
		Priority: 1, Desc: "Galileo E5b I",
	},
	{2, 6}: {
		GnssID: 2, SigID: 6, FreqBand: 2,
		Rinex3:   Rinex3Codes{"C7Q", "L7Q", "D7Q", "S7Q"},
		Priority: 2, Desc: "Galileo E5b Q",
	},

	// ── BeiDou (gnssId=3) ───────────────────────────────────────────
	{3, 0}: {
		GnssID: 3, SigID: 0, FreqBand: 0,
		Rinex3:   Rinex3Codes{"C2I", "L2I", "D2I", "S2I"},
		Priority: 1, Desc: "BeiDou B1I D1",
	},
	{3, 1}: {
		GnssID: 3, SigID: 1, FreqBand: 0,
		Rinex3:   Rinex3Codes{"C2I", "L2I", "D2I", "S2I"},
		Priority: 2, Desc: "BeiDou B1I D2",
	},
	{3, 2}: {
		GnssID: 3, SigID: 2, FreqBand: 2,
		Rinex3:   Rinex3Codes{"C7I", "L7I", "D7I", "S7I"},
		Priority: 1, Desc: "BeiDou B2I D1",
	},
	{3, 3}: {
		GnssID: 3, SigID: 3, FreqBand: 2,
		Rinex3:   Rinex3Codes{"C7I", "L7I", "D7I", "S7I"},
		Priority: 2, Desc: "BeiDou B2I D2",
	},
}

// LookupMapping returns the signal mapping for a given UBX gnssId and sigId.
func LookupMapping(gnssId, sigId uint8) (SignalMapping, bool) {
	m, ok := signalTable[signalKey{gnssId, sigId}]
	return m, ok
}

// BestSignalPerBand selects the highest-priority signal per frequency band
// from a satellite's signal list. Used when RINEX 2.11 can only represent
// one signal per frequency.
func BestSignalPerBand(signals []gnss.Signal) map[uint8]gnss.Signal {
	best := make(map[uint8]gnss.Signal)
	bestPri := make(map[uint8]int) // tracks priority of current best

	for _, sig := range signals {
		m, ok := LookupMapping(sig.GnssID, sig.SigID)
		if !ok {
			continue
		}
		band := m.FreqBand
		cur, exists := bestPri[band]
		if !exists || m.Priority < cur {
			best[band] = sig
			bestPri[band] = m.Priority
		}
	}
	return best
}

// Rinex2ObsTypes returns the ordered list of RINEX 2.11 observation types
// for GPS-only output.
func Rinex2ObsTypes() []string {
	return []string{"C1", "C2", "L1", "L2", "D1", "D2", "S1", "S2"}
}

// Rinex3ObsTypes returns RINEX 3.x observation types for a given constellation.
// The returned codes are sorted and deduplicated.
func Rinex3ObsTypes(gnssId uint8) []string {
	seen := make(map[string]bool)
	var codes []string

	for k, m := range signalTable {
		if k.gnssID != gnssId {
			continue
		}
		for _, c := range []string{m.Rinex3.Pseudorange, m.Rinex3.Phase, m.Rinex3.Doppler, m.Rinex3.SNR} {
			if c != "" && !seen[c] {
				seen[c] = true
				codes = append(codes, c)
			}
		}
	}
	sort.Strings(codes)
	return codes
}
