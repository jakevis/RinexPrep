package ubx

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestExtractBits(t *testing.T) {
	// 30-bit word: 0x3FFFFFFF (all 1s in 30 bits)
	allOnes := uint32(0x3FFFFFFF)

	// Extract all 30 bits
	if got := extractBits(allOnes, 0, 30); got != 0x3FFFFFFF {
		t.Errorf("extractBits(all ones, 0, 30) = 0x%X, want 0x3FFFFFFF", got)
	}

	// Extract top 10 bits
	if got := extractBits(allOnes, 0, 10); got != 0x3FF {
		t.Errorf("extractBits(all ones, 0, 10) = 0x%X, want 0x3FF", got)
	}

	// Specific pattern: word = 0x12345678 (only lower 30 bits matter)
	word := uint32(0x12345678)
	// Bits 0-7 (top 8 bits of the 30-bit value)
	got := extractBits(word, 0, 8)
	expected := (word >> 22) & 0xFF
	if got != expected {
		t.Errorf("extractBits(0x%X, 0, 8) = 0x%X, want 0x%X", word, got, expected)
	}

	// Bits 8-15 (next 8 bits)
	got = extractBits(word, 8, 8)
	expected = (word >> 14) & 0xFF
	if got != expected {
		t.Errorf("extractBits(0x%X, 8, 8) = 0x%X, want 0x%X", word, got, expected)
	}
}

func TestTwosComp(t *testing.T) {
	tests := []struct {
		val  uint32
		bits int
		want int32
	}{
		{0, 8, 0},
		{127, 8, 127},
		{128, 8, -128},
		{255, 8, -1},
		{0x7FFF, 16, 32767},
		{0x8000, 16, -32768},
		{0xFFFF, 16, -1},
		{1, 8, 1},
		{0xFE, 8, -2},
	}

	for _, tc := range tests {
		got := twosComp(tc.val, tc.bits)
		if got != tc.want {
			t.Errorf("twosComp(0x%X, %d) = %d, want %d", tc.val, tc.bits, got, tc.want)
		}
	}
}

// buildSfrbxPayload creates a valid RXM-SFRBX payload.
func buildSfrbxPayload(gnssID, svID, freqID, numWords, channel, version uint8, words []uint32) []byte {
	payload := make([]byte, sfrbxHeaderSize+int(numWords)*4)
	payload[0] = gnssID
	payload[1] = svID
	payload[2] = freqID
	payload[3] = numWords
	payload[4] = channel
	payload[5] = version
	// payload[6:8] reserved

	for i, w := range words {
		off := sfrbxHeaderSize + i*4
		binary.LittleEndian.PutUint32(payload[off:off+4], w)
	}
	return payload
}

func TestDecodeSfrbx(t *testing.T) {
	words := []uint32{0x11223344, 0x55667788, 0x99AABBCC, 0xDDEEFF00, 0x12345678,
		0x9ABCDEF0, 0x13579BDF, 0x2468ACE0, 0xFEDCBA98, 0x76543210}
	payload := buildSfrbxPayload(0, 12, 0, 10, 5, 2, words)

	msg, err := decodeSfrbx(payload)
	if err != nil {
		t.Fatalf("decodeSfrbx error: %v", err)
	}

	if msg.GnssID != 0 {
		t.Errorf("GnssID = %d, want 0", msg.GnssID)
	}
	if msg.SvID != 12 {
		t.Errorf("SvID = %d, want 12", msg.SvID)
	}
	if msg.NumWords != 10 {
		t.Errorf("NumWords = %d, want 10", msg.NumWords)
	}
	if msg.Channel != 5 {
		t.Errorf("Channel = %d, want 5", msg.Channel)
	}
	if msg.Version != 2 {
		t.Errorf("Version = %d, want 2", msg.Version)
	}
	if len(msg.Words) != 10 {
		t.Fatalf("len(Words) = %d, want 10", len(msg.Words))
	}
	for i, w := range words {
		if msg.Words[i] != w {
			t.Errorf("Words[%d] = 0x%X, want 0x%X", i, msg.Words[i], w)
		}
	}
}

func TestDecodeSfrbxTooShort(t *testing.T) {
	// Payload shorter than header
	_, err := decodeSfrbx(make([]byte, 4))
	if err == nil {
		t.Error("expected error for short payload")
	}

	// Header says 10 words but payload is too short
	shortPayload := make([]byte, sfrbxHeaderSize+5*4)
	shortPayload[3] = 10 // numWords = 10 but only space for 5
	_, err = decodeSfrbx(shortPayload)
	if err == nil {
		t.Error("expected error for insufficient word data")
	}
}

func TestSubframeIDExtraction(t *testing.T) {
	// Build words where word 2 (HOW) has subframe ID in bits 8-10
	for sfID := uint32(1); sfID <= 5; sfID++ {
		words := make([]uint32, 10)
		words[1] = sfID << 8 // Place subframe ID in bits 8-10

		msg := &SfrbxMessage{
			GnssID:   0,
			NumWords: 10,
			Words:    words,
		}

		ephem := &GPSEphemeris{}
		ParseGPSSubframe(msg, ephem)

		switch sfID {
		case 1:
			if !ephem.HasSF1 {
				t.Errorf("sfID=%d: expected HasSF1=true", sfID)
			}
		case 2:
			if !ephem.HasSF2 {
				t.Errorf("sfID=%d: expected HasSF2=true", sfID)
			}
		case 3:
			if !ephem.HasSF3 {
				t.Errorf("sfID=%d: expected HasSF3=true", sfID)
			}
		case 4, 5:
			// Subframes 4 and 5 are not parsed for ephemeris
			if ephem.HasSF1 || ephem.HasSF2 || ephem.HasSF3 {
				t.Errorf("sfID=%d: expected no ephemeris flags set", sfID)
			}
		}
	}
}

func TestNonGPSIgnored(t *testing.T) {
	words := make([]uint32, 10)
	words[1] = 1 << 8 // subframe 1

	msg := &SfrbxMessage{
		GnssID:   2, // Galileo, not GPS
		NumWords: 10,
		Words:    words,
	}

	ephem := &GPSEphemeris{}
	ParseGPSSubframe(msg, ephem)
	if ephem.HasSF1 || ephem.HasSF2 || ephem.HasSF3 {
		t.Error("expected no parsing for non-GPS GNSS ID")
	}
}

func TestTooFewWordsIgnored(t *testing.T) {
	msg := &SfrbxMessage{
		GnssID:   0,
		NumWords: 5,
		Words:    make([]uint32, 5),
	}
	ephem := &GPSEphemeris{}
	ParseGPSSubframe(msg, ephem)
	if ephem.HasSF1 || ephem.HasSF2 || ephem.HasSF3 {
		t.Error("expected no parsing for fewer than 10 words")
	}
}

// buildGPSSubframeWords creates words for a given subframe with known parameter values.
func buildSubframe1Words(weekNum uint16, uraIdx, svHealth uint8, iodc uint16,
	tgd int32, toc uint32, af2 int32, af1 int32, af0 int32) []uint32 {

	words := make([]uint32, 10)
	// Word 2 (HOW): subframe ID = 1 in bits 8-10
	words[1] = 1 << 8

	// Word 3: WN(0-9), codesL2(10-11), URA(12-15), SVHealth(16-21), IODC MSBs(22-23)
	w3 := uint32(weekNum&0x3FF) << 20 // bits 0-9
	w3 |= uint32(uraIdx&0x0F) << 14   // bits 12-15
	w3 |= uint32(svHealth&0x3F) << 8  // bits 16-21
	w3 |= uint32((iodc>>8)&0x03) << 6 // bits 22-23
	words[2] = w3

	// Word 7: TGD in bits 16-23
	words[6] = uint32(tgd&0xFF) << 6

	// Word 8: IODC LSBs (bits 0-7), toc (bits 8-23)
	w8 := uint32(iodc&0xFF) << 22
	w8 |= uint32(toc&0xFFFF) << 6
	words[7] = w8

	// Word 9: af2 (bits 0-7), af1 (bits 8-23)
	w9 := uint32(af2&0xFF) << 22
	w9 |= uint32(af1&0xFFFF) << 6
	words[8] = w9

	// Word 10: af0 (bits 0-21)
	words[9] = uint32(af0&0x3FFFFF) << 8

	return words
}

func TestParseSubframe1(t *testing.T) {
	// Known values
	weekNum := uint16(100)
	uraIdx := uint8(3)
	svHealth := uint8(0)
	iodc := uint16(0x0AB) // 10 bits, MSBs = 0x02, LSBs = 0xAB
	tgdRaw := int32(-5)
	tocRaw := uint32(7200) // * 2^4 = 115200 seconds
	af2Raw := int32(0)
	af1Raw := int32(-100)
	af0Raw := int32(50000)

	words := buildSubframe1Words(weekNum, uraIdx, svHealth, iodc, tgdRaw, tocRaw, af2Raw, af1Raw, af0Raw)
	msg := &SfrbxMessage{GnssID: 0, NumWords: 10, Words: words}

	ephem := &GPSEphemeris{}
	ParseGPSSubframe(msg, ephem)

	if !ephem.HasSF1 {
		t.Fatal("expected HasSF1 = true")
	}
	if ephem.WeekNum != weekNum {
		t.Errorf("WeekNum = %d, want %d", ephem.WeekNum, weekNum)
	}
	if ephem.SVAccuracy != uraIdx {
		t.Errorf("SVAccuracy = %d, want %d", ephem.SVAccuracy, uraIdx)
	}
	if ephem.SVHealth != svHealth {
		t.Errorf("SVHealth = %d, want %d", ephem.SVHealth, svHealth)
	}
	if ephem.IODC != iodc {
		t.Errorf("IODC = 0x%X, want 0x%X", ephem.IODC, iodc)
	}

	expectedTGD := float64(tgdRaw) * math.Exp2(-31)
	if math.Abs(ephem.TGD-expectedTGD) > 1e-20 {
		t.Errorf("TGD = %e, want %e", ephem.TGD, expectedTGD)
	}

	expectedToc := float64(tocRaw) * math.Exp2(4)
	if math.Abs(ephem.Toc-expectedToc) > 1e-9 {
		t.Errorf("Toc = %f, want %f", ephem.Toc, expectedToc)
	}

	expectedAf2 := float64(af2Raw) * math.Exp2(-55)
	if math.Abs(ephem.Af2-expectedAf2) > 1e-30 {
		t.Errorf("Af2 = %e, want %e", ephem.Af2, expectedAf2)
	}

	expectedAf1 := float64(af1Raw) * math.Exp2(-43)
	if math.Abs(ephem.Af1-expectedAf1) > 1e-25 {
		t.Errorf("Af1 = %e, want %e", ephem.Af1, expectedAf1)
	}

	expectedAf0 := float64(af0Raw) * math.Exp2(-31)
	if math.Abs(ephem.Af0-expectedAf0) > 1e-20 {
		t.Errorf("Af0 = %e, want %e", ephem.Af0, expectedAf0)
	}
}

func TestEphemerisCompleteness(t *testing.T) {
	ephem := &GPSEphemeris{}

	if ephem.IsComplete() {
		t.Error("empty ephemeris should not be complete")
	}

	// Send subframe 1
	words1 := make([]uint32, 10)
	words1[1] = 1 << 8
	ParseGPSSubframe(&SfrbxMessage{GnssID: 0, NumWords: 10, Words: words1}, ephem)
	if ephem.IsComplete() {
		t.Error("ephemeris with only SF1 should not be complete")
	}

	// Send subframe 2
	words2 := make([]uint32, 10)
	words2[1] = 2 << 8
	ParseGPSSubframe(&SfrbxMessage{GnssID: 0, NumWords: 10, Words: words2}, ephem)
	if ephem.IsComplete() {
		t.Error("ephemeris with SF1+SF2 should not be complete")
	}

	// Send subframe 3
	words3 := make([]uint32, 10)
	words3[1] = 3 << 8
	ParseGPSSubframe(&SfrbxMessage{GnssID: 0, NumWords: 10, Words: words3}, ephem)
	if !ephem.IsComplete() {
		t.Error("ephemeris with SF1+SF2+SF3 should be complete")
	}
}

func TestSfrbxInParser(t *testing.T) {
	// Build an SFRBX frame for GPS PRN 5, subframe 1
	words := make([]uint32, 10)
	words[1] = 1 << 8 // subframe ID = 1
	payload := buildSfrbxPayload(0, 5, 0, 10, 1, 2, words)
	frame := buildUBXFrame(ClassRXM, IDSfrbx, payload)

	_, stats, err := Parse(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stats.TotalMessages != 1 {
		t.Errorf("TotalMessages = %d, want 1", stats.TotalMessages)
	}
	if stats.SfrbxMessages != 1 {
		t.Errorf("SfrbxMessages = %d, want 1", stats.SfrbxMessages)
	}
	if stats.Ephemerides == nil {
		t.Fatal("Ephemerides map is nil")
	}
	ephem, ok := stats.Ephemerides[5]
	if !ok {
		t.Fatal("no ephemeris for PRN 5")
	}
	if !ephem.HasSF1 {
		t.Error("expected HasSF1 = true for PRN 5")
	}
	if ephem.PRN != 5 {
		t.Errorf("PRN = %d, want 5", ephem.PRN)
	}
}

func TestSfrbxNonGPSNotStored(t *testing.T) {
	// Galileo SFRBX should be counted but not stored in Ephemerides
	words := make([]uint32, 10)
	payload := buildSfrbxPayload(2, 3, 0, 10, 1, 2, words)
	frame := buildUBXFrame(ClassRXM, IDSfrbx, payload)

	_, stats, err := Parse(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stats.SfrbxMessages != 1 {
		t.Errorf("SfrbxMessages = %d, want 1", stats.SfrbxMessages)
	}
	if stats.Ephemerides != nil && len(stats.Ephemerides) > 0 {
		t.Error("expected no ephemeris entries for non-GPS")
	}
}

func TestSfrbxMultipleSubframesSamePRN(t *testing.T) {
	var input []byte
	for sfID := uint32(1); sfID <= 3; sfID++ {
		words := make([]uint32, 10)
		words[1] = sfID << 8
		payload := buildSfrbxPayload(0, 7, 0, 10, 1, 2, words)
		input = append(input, buildUBXFrame(ClassRXM, IDSfrbx, payload)...)
	}

	_, stats, err := Parse(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stats.SfrbxMessages != 3 {
		t.Errorf("SfrbxMessages = %d, want 3", stats.SfrbxMessages)
	}
	ephem, ok := stats.Ephemerides[7]
	if !ok {
		t.Fatal("no ephemeris for PRN 7")
	}
	if !ephem.IsComplete() {
		t.Error("expected complete ephemeris after SF1+SF2+SF3")
	}
}
