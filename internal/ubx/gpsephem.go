package ubx

import "math"

// GPSEphemeris contains orbital parameters from GPS broadcast ephemeris.
type GPSEphemeris struct {
	PRN uint8

	// Subframe 1
	WeekNum     uint16  // GPS week number (10-bit, mod 1024)
	SVAccuracy  uint8   // SV accuracy (URA index)
	SVHealth    uint8   // SV health
	IODC        uint16  // Issue of Data Clock
	TGD         float64 // Group delay (seconds)
	Toc         float64 // Clock correction time (seconds)
	Af2         float64 // Clock drift rate (s/s^2)
	Af1         float64 // Clock drift (s/s)
	Af0         float64 // Clock bias (s)

	// Subframe 2
	IODE2       uint8   // Issue of Data Ephemeris (SF2)
	Crs         float64 // Sine correction to orbital radius (m)
	DeltaN      float64 // Mean motion difference (rad/s)
	M0          float64 // Mean anomaly at reference time (rad)
	Cuc         float64 // Cosine correction to argument of latitude (rad)
	Ecc         float64 // Eccentricity
	Cus         float64 // Sine correction to argument of latitude (rad)
	SqrtA       float64 // Square root of semi-major axis (m^1/2)
	Toe         float64 // Reference time of ephemeris (s)
	FitInterval uint8   // Fit interval flag

	// Subframe 3
	IODE3    uint8   // Issue of Data Ephemeris (SF3)
	Cic      float64 // Cosine correction to inclination (rad)
	Omega0   float64 // Longitude of ascending node (rad)
	Cis      float64 // Sine correction to inclination (rad)
	I0       float64 // Inclination at reference time (rad)
	Crc      float64 // Cosine correction to orbital radius (m)
	Omega    float64 // Argument of perigee (rad)
	OmegaDot float64 // Rate of right ascension (rad/s)
	IDOT     float64 // Rate of inclination (rad/s)

	// Completeness tracking
	HasSF1 bool
	HasSF2 bool
	HasSF3 bool
}

// IsComplete returns true if all three subframes have been received.
func (e *GPSEphemeris) IsComplete() bool {
	return e.HasSF1 && e.HasSF2 && e.HasSF3
}

// extractBits extracts bits from a 30-bit GPS word.
// GPS ICD convention: bit 1 is LSB of the 30-bit word (before parity),
// MSB is bit 30. startBit is numbered from the MSB (bit 30 = position 0).
func extractBits(word uint32, startBit, numBits int) uint32 {
	shift := 30 - startBit - numBits
	if shift < 0 {
		shift = 0
	}
	mask := uint32((1 << numBits) - 1)
	return (word >> shift) & mask
}

// twosComp converts an unsigned value to signed using two's complement.
func twosComp(val uint32, bits int) int32 {
	if val >= (1 << (bits - 1)) {
		return int32(val) - (1 << bits)
	}
	return int32(val)
}

// ParseGPSSubframe extracts ephemeris data from a GPS SFRBX message.
func ParseGPSSubframe(msg *SfrbxMessage, ephem *GPSEphemeris) {
	if msg.GnssID != 0 || len(msg.Words) < 10 {
		return
	}

	// Extract subframe ID from word 2 (HOW), bits 8-10.
	// In the HOW word, the subframe ID is in bits 20-22 (MSB numbering from 1).
	// With our extractBits using 0-based MSB: bits at positions 8,9,10 → startBit=8, numBits=3.
	sfID := (msg.Words[1] >> 8) & 0x07

	switch sfID {
	case 1:
		parseSubframe1(msg.Words, ephem)
	case 2:
		parseSubframe2(msg.Words, ephem)
	case 3:
		parseSubframe3(msg.Words, ephem)
	}
}

// parseSubframe1 extracts clock and satellite parameters from GPS subframe 1.
// Reference: IS-GPS-200 Section 20.3.3.3 (Table 20-I).
func parseSubframe1(words []uint32, e *GPSEphemeris) {
	// Word 3: WN (bits 1-10), codes on L2 (bits 11-12), URA (bits 13-16), SV health (bits 17-22), IODC MSBs (bits 23-24)
	e.WeekNum = uint16(extractBits(words[2], 0, 10))
	e.SVAccuracy = uint8(extractBits(words[2], 12, 4))
	e.SVHealth = uint8(extractBits(words[2], 16, 6))
	iodcMSB := extractBits(words[2], 22, 2)

	// Word 7: TGD (bits 17-24)
	tgdRaw := extractBits(words[6], 16, 8)
	e.TGD = float64(twosComp(tgdRaw, 8)) * math.Exp2(-31)

	// Word 8: IODC LSBs (bits 1-8), toc (bits 9-24)
	iodcLSB := extractBits(words[7], 0, 8)
	e.IODC = uint16(iodcMSB)<<8 | uint16(iodcLSB)
	tocRaw := extractBits(words[7], 8, 16)
	e.Toc = float64(tocRaw) * math.Exp2(4)

	// Word 9: af2 (bits 1-8), af1 (bits 9-24)
	af2Raw := extractBits(words[8], 0, 8)
	e.Af2 = float64(twosComp(af2Raw, 8)) * math.Exp2(-55)
	af1Raw := extractBits(words[8], 8, 16)
	e.Af1 = float64(twosComp(af1Raw, 16)) * math.Exp2(-43)

	// Word 10: af0 (bits 1-22)
	af0Raw := extractBits(words[9], 0, 22)
	e.Af0 = float64(twosComp(af0Raw, 22)) * math.Exp2(-31)

	e.HasSF1 = true
}

// parseSubframe2 extracts ephemeris parameters from GPS subframe 2.
// Reference: IS-GPS-200 Section 20.3.3.4 (Table 20-II).
func parseSubframe2(words []uint32, e *GPSEphemeris) {
	// Word 3: IODE (bits 1-8), Crs (bits 9-24)
	e.IODE2 = uint8(extractBits(words[2], 0, 8))
	crsRaw := extractBits(words[2], 8, 16)
	e.Crs = float64(twosComp(crsRaw, 16)) * math.Exp2(-5)

	// Word 4: Delta n (bits 1-16), M0 MSBs (bits 17-24)
	dnRaw := extractBits(words[3], 0, 16)
	e.DeltaN = float64(twosComp(dnRaw, 16)) * math.Exp2(-43) * math.Pi

	// Word 5: M0 LSBs (bits 1-24)
	m0MSB := extractBits(words[3], 16, 8)
	m0LSB := extractBits(words[4], 0, 24)
	m0Raw := (m0MSB << 24) | m0LSB
	e.M0 = float64(twosComp(m0Raw, 32)) * math.Exp2(-31) * math.Pi

	// Word 6: Cuc (bits 1-16), ecc MSBs (bits 17-24)
	cucRaw := extractBits(words[5], 0, 16)
	e.Cuc = float64(twosComp(cucRaw, 16)) * math.Exp2(-29)

	// Word 7: ecc LSBs (bits 1-24)
	eccMSB := extractBits(words[5], 16, 8)
	eccLSB := extractBits(words[6], 0, 24)
	eccRaw := (eccMSB << 24) | eccLSB
	e.Ecc = float64(eccRaw) * math.Exp2(-33) // unsigned

	// Word 8: Cus (bits 1-16), sqrtA MSBs (bits 17-24)
	cusRaw := extractBits(words[7], 0, 16)
	e.Cus = float64(twosComp(cusRaw, 16)) * math.Exp2(-29)

	// Word 9: sqrtA LSBs (bits 1-24)
	sqrtAMSB := extractBits(words[7], 16, 8)
	sqrtALSB := extractBits(words[8], 0, 24)
	sqrtARaw := (sqrtAMSB << 24) | sqrtALSB
	e.SqrtA = float64(sqrtARaw) * math.Exp2(-19) // unsigned

	// Word 10: toe (bits 1-16), fit interval (bit 17)
	toeRaw := extractBits(words[9], 0, 16)
	e.Toe = float64(toeRaw) * math.Exp2(4)
	e.FitInterval = uint8(extractBits(words[9], 16, 1))

	e.HasSF2 = true
}

// parseSubframe3 extracts ephemeris parameters from GPS subframe 3.
// Reference: IS-GPS-200 Section 20.3.3.4 (Table 20-III).
func parseSubframe3(words []uint32, e *GPSEphemeris) {
	// Word 3: Cic (bits 1-16), Omega0 MSBs (bits 17-24)
	cicRaw := extractBits(words[2], 0, 16)
	e.Cic = float64(twosComp(cicRaw, 16)) * math.Exp2(-29)

	// Word 4: Omega0 LSBs (bits 1-24)
	omega0MSB := extractBits(words[2], 16, 8)
	omega0LSB := extractBits(words[3], 0, 24)
	omega0Raw := (omega0MSB << 24) | omega0LSB
	e.Omega0 = float64(twosComp(omega0Raw, 32)) * math.Exp2(-31) * math.Pi

	// Word 5: Cis (bits 1-16), i0 MSBs (bits 17-24)
	cisRaw := extractBits(words[4], 0, 16)
	e.Cis = float64(twosComp(cisRaw, 16)) * math.Exp2(-29)

	// Word 6: i0 LSBs (bits 1-24)
	i0MSB := extractBits(words[4], 16, 8)
	i0LSB := extractBits(words[5], 0, 24)
	i0Raw := (i0MSB << 24) | i0LSB
	e.I0 = float64(twosComp(i0Raw, 32)) * math.Exp2(-31) * math.Pi

	// Word 7: Crc (bits 1-16), omega MSBs (bits 17-24)
	crcRaw := extractBits(words[6], 0, 16)
	e.Crc = float64(twosComp(crcRaw, 16)) * math.Exp2(-5)

	// Word 8: omega LSBs (bits 1-24)
	omegaMSB := extractBits(words[6], 16, 8)
	omegaLSB := extractBits(words[7], 0, 24)
	omegaRaw := (omegaMSB << 24) | omegaLSB
	e.Omega = float64(twosComp(omegaRaw, 32)) * math.Exp2(-31) * math.Pi

	// Word 9: OmegaDot (bits 1-24)
	omegaDotRaw := extractBits(words[8], 0, 24)
	e.OmegaDot = float64(twosComp(omegaDotRaw, 24)) * math.Exp2(-43) * math.Pi

	// Word 10: IODE (bits 1-8), IDOT (bits 9-22)
	e.IODE3 = uint8(extractBits(words[9], 0, 8))
	idotRaw := extractBits(words[9], 8, 14)
	e.IDOT = float64(twosComp(idotRaw, 14)) * math.Exp2(-43) * math.Pi

	e.HasSF3 = true
}
