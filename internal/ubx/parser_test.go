package ubx

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// buildUBXFrame constructs a complete UBX frame (sync + class + id + length + payload + checksum).
func buildUBXFrame(class, id byte, payload []byte) []byte {
	payloadLen := len(payload)
	// class(1) + id(1) + length(2) + payload
	checksumData := make([]byte, 4+payloadLen)
	checksumData[0] = class
	checksumData[1] = id
	binary.LittleEndian.PutUint16(checksumData[2:4], uint16(payloadLen))
	copy(checksumData[4:], payload)

	ckA, ckB := Checksum(checksumData)

	frame := make([]byte, 0, 2+len(checksumData)+2)
	frame = append(frame, SyncByte1, SyncByte2)
	frame = append(frame, checksumData...)
	frame = append(frame, ckA, ckB)
	return frame
}

// buildRawxPayload creates a valid RXM-RAWX payload with the given parameters.
func buildRawxPayload(rcvTow float64, week uint16, leapS int8, recStat uint8, meas []RawxMeas) []byte {
	numMeas := len(meas)
	payload := make([]byte, rawxHeaderSize+numMeas*rawxMeasSize)

	binary.LittleEndian.PutUint64(payload[0:8], math.Float64bits(rcvTow))
	binary.LittleEndian.PutUint16(payload[8:10], week)
	payload[10] = byte(leapS)
	payload[11] = byte(numMeas)
	payload[12] = recStat
	// bytes 13-15 reserved

	for i, m := range meas {
		off := rawxHeaderSize + i*rawxMeasSize
		binary.LittleEndian.PutUint64(payload[off+0:off+8], math.Float64bits(m.PrMes))
		binary.LittleEndian.PutUint64(payload[off+8:off+16], math.Float64bits(m.CpMes))
		binary.LittleEndian.PutUint32(payload[off+16:off+20], math.Float32bits(m.DoMes))
		payload[off+20] = m.GnssID
		payload[off+21] = m.SvID
		payload[off+22] = m.SigID
		payload[off+23] = m.FreqID
		binary.LittleEndian.PutUint16(payload[off+24:off+26], m.Locktime)
		payload[off+26] = m.Cno
		payload[off+27] = m.PrStDev
		payload[off+28] = m.CpStDev
		payload[off+29] = m.DoStDev
		payload[off+30] = m.TrkStat
		// byte 31 reserved
	}
	return payload
}

func TestChecksumKnownData(t *testing.T) {
	// UBX checksum is Fletcher-8 over the data.
	data := []byte{0x02, 0x15, 0x04, 0x00, 0xAA, 0xBB, 0xCC, 0xDD}
	ckA, ckB := Checksum(data)

	// Verify manually: compute expected values.
	var a, b uint8
	for _, v := range data {
		a += v
		b += a
	}
	if ckA != a || ckB != b {
		t.Errorf("Checksum mismatch: got (%d, %d), want (%d, %d)", ckA, ckB, a, b)
	}

	if !ValidChecksum(data, ckA, ckB) {
		t.Error("ValidChecksum returned false for correct checksum")
	}

	if ValidChecksum(data, ckA+1, ckB) {
		t.Error("ValidChecksum returned true for incorrect ckA")
	}

	if ValidChecksum(data, ckA, ckB+1) {
		t.Error("ValidChecksum returned true for incorrect ckB")
	}
}

func TestChecksumEmpty(t *testing.T) {
	ckA, ckB := Checksum(nil)
	if ckA != 0 || ckB != 0 {
		t.Errorf("Checksum of empty data: got (%d, %d), want (0, 0)", ckA, ckB)
	}
}

func TestSyncByteScanningWithGarbage(t *testing.T) {
	// Build a valid frame with some known payload.
	payload := buildRawxPayload(100000.0, 2200, 18, 0x03, []RawxMeas{
		{
			PrMes: 22000000.0, CpMes: 115000000.0, DoMes: -1500.0,
			GnssID: 0, SvID: 5, SigID: 0, FreqID: 0,
			Locktime: 30000, Cno: 42, TrkStat: 0x03,
		},
	})
	frame := buildUBXFrame(ClassRXM, IDRawx, payload)

	// Prepend garbage data.
	garbage := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0xFF, 0xB5, 0x00}
	input := append(garbage, frame...)

	epochs, stats, err := Parse(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stats.SkippedBytes == 0 {
		t.Error("expected SkippedBytes > 0 for garbage prefix")
	}

	if len(epochs) != 1 {
		t.Fatalf("expected 1 epoch, got %d", len(epochs))
	}

	if stats.RawxMessages != 1 {
		t.Errorf("expected 1 RawxMessages, got %d", stats.RawxMessages)
	}
}

func TestRawxDecode(t *testing.T) {
	rcvTow := 259200.123456
	week := uint16(2300)
	leapS := int8(18)
	recStat := uint8(0x03) // leapSec valid + clkReset

	meas := []RawxMeas{
		{
			PrMes: 22000000.5, CpMes: 115600000.25, DoMes: -1234.5,
			GnssID: 0, SvID: 12, SigID: 0, FreqID: 0,
			Locktime: 50000, Cno: 45, TrkStat: 0x03,
		},
		{
			PrMes: 23000000.75, CpMes: 120000000.5, DoMes: 567.25,
			GnssID: 2, SvID: 3, SigID: 1, FreqID: 0,
			Locktime: 10000, Cno: 38, TrkStat: 0x01, // prValid only
		},
		{
			PrMes: 24000000.0, CpMes: 125000000.0, DoMes: -200.0,
			GnssID: 0, SvID: 12, SigID: 1, FreqID: 0, // Same sat as first, different signal
			Locktime: 45000, Cno: 40, TrkStat: 0x03,
		},
	}

	payload := buildRawxPayload(rcvTow, week, leapS, recStat, meas)
	frame := buildUBXFrame(ClassRXM, IDRawx, payload)

	epochs, stats, err := Parse(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stats.TotalMessages != 1 {
		t.Errorf("TotalMessages = %d, want 1", stats.TotalMessages)
	}
	if stats.RawxMessages != 1 {
		t.Errorf("RawxMessages = %d, want 1", stats.RawxMessages)
	}
	if stats.ChecksumErrors != 0 {
		t.Errorf("ChecksumErrors = %d, want 0", stats.ChecksumErrors)
	}

	if len(epochs) != 1 {
		t.Fatalf("expected 1 epoch, got %d", len(epochs))
	}

	ep := epochs[0]

	// Check time.
	if ep.Time.Week != week {
		t.Errorf("Week = %d, want %d", ep.Time.Week, week)
	}
	expectedTOW := int64(rcvTow * 1e9)
	if ep.Time.TOWNanos != expectedTOW {
		t.Errorf("TOWNanos = %d, want %d", ep.Time.TOWNanos, expectedTOW)
	}
	if ep.Time.LeapSeconds != leapS {
		t.Errorf("LeapSeconds = %d, want %d", ep.Time.LeapSeconds, leapS)
	}
	if !ep.Time.LeapValid {
		t.Error("expected LeapValid=true")
	}
	if !ep.Time.ClkReset {
		t.Error("expected ClkReset=true")
	}

	// 3 measurements but G12 has 2 signals → 2 SatObs.
	if len(ep.Satellites) != 2 {
		t.Fatalf("expected 2 satellites, got %d", len(ep.Satellites))
	}

	// First satellite: GPS PRN 12, 2 signals.
	sat0 := ep.Satellites[0]
	if sat0.Constellation != gnss.ConsGPS {
		t.Errorf("sat0 constellation = %d, want GPS(%d)", sat0.Constellation, gnss.ConsGPS)
	}
	if sat0.PRN != 12 {
		t.Errorf("sat0 PRN = %d, want 12", sat0.PRN)
	}
	if len(sat0.Signals) != 2 {
		t.Fatalf("sat0 signals = %d, want 2", len(sat0.Signals))
	}
	if sat0.Signals[0].Pseudorange != 22000000.5 {
		t.Errorf("sig0 pseudorange = %f, want 22000000.5", sat0.Signals[0].Pseudorange)
	}
	if !sat0.Signals[0].PRValid || !sat0.Signals[0].CPValid {
		t.Error("sig0 expected prValid and cpValid")
	}
	if sat0.Signals[1].SigID != 1 {
		t.Errorf("sig1 sigID = %d, want 1", sat0.Signals[1].SigID)
	}

	// Second satellite: Galileo PRN 3, 1 signal.
	sat1 := ep.Satellites[1]
	if sat1.Constellation != gnss.ConsGalileo {
		t.Errorf("sat1 constellation = %d, want Galileo(%d)", sat1.Constellation, gnss.ConsGalileo)
	}
	if sat1.PRN != 3 {
		t.Errorf("sat1 PRN = %d, want 3", sat1.PRN)
	}
	if len(sat1.Signals) != 1 {
		t.Fatalf("sat1 signals = %d, want 1", len(sat1.Signals))
	}
	if sat1.Signals[0].PRValid != true {
		t.Error("sat1 sig0 expected prValid")
	}
	if sat1.Signals[0].CPValid != false {
		t.Error("sat1 sig0 expected cpValid=false")
	}
	if sat1.Signals[0].SNR != 38 {
		t.Errorf("sat1 sig0 SNR = %f, want 38", sat1.Signals[0].SNR)
	}
	if sat1.SatID() != "E03" {
		t.Errorf("sat1 SatID = %q, want E03", sat1.SatID())
	}
}

func TestTruncatedMessage(t *testing.T) {
	payload := buildRawxPayload(100000.0, 2200, 18, 0x01, []RawxMeas{
		{GnssID: 0, SvID: 1, TrkStat: 0x03},
	})
	frame := buildUBXFrame(ClassRXM, IDRawx, payload)

	// Truncate the frame (remove last 10 bytes).
	truncated := frame[:len(frame)-10]

	epochs, stats, err := Parse(bytes.NewReader(truncated))
	if err != nil {
		t.Fatalf("Parse error on truncated input: %v", err)
	}

	// Should gracefully return with no epochs (truncated frame discarded).
	if len(epochs) != 0 {
		t.Errorf("expected 0 epochs for truncated input, got %d", len(epochs))
	}
	_ = stats
}

func TestBadChecksumSkipsAndContinues(t *testing.T) {
	// Build two valid frames, corrupt the checksum of the first.
	payload1 := buildRawxPayload(100000.0, 2200, 18, 0x01, []RawxMeas{
		{PrMes: 22000000.0, GnssID: 0, SvID: 1, TrkStat: 0x03},
	})
	frame1 := buildUBXFrame(ClassRXM, IDRawx, payload1)
	// Corrupt checksum of frame1.
	frame1[len(frame1)-1] ^= 0xFF
	frame1[len(frame1)-2] ^= 0xFF

	payload2 := buildRawxPayload(100030.0, 2200, 18, 0x01, []RawxMeas{
		{PrMes: 23000000.0, GnssID: 0, SvID: 2, TrkStat: 0x03},
	})
	frame2 := buildUBXFrame(ClassRXM, IDRawx, payload2)

	input := append(frame1, frame2...)

	epochs, stats, err := Parse(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stats.ChecksumErrors != 1 {
		t.Errorf("ChecksumErrors = %d, want 1", stats.ChecksumErrors)
	}

	if len(epochs) != 1 {
		t.Fatalf("expected 1 epoch (second frame), got %d", len(epochs))
	}

	if epochs[0].Satellites[0].PRN != 2 {
		t.Errorf("expected PRN=2 from second frame, got %d", epochs[0].Satellites[0].PRN)
	}
}

func TestNonRawxMessagesAreCounted(t *testing.T) {
	// Build a non-RAWX message (e.g., NAV-PVT: class=0x01, id=0x07).
	payload := make([]byte, 10) // arbitrary small payload
	frame := buildUBXFrame(0x01, 0x07, payload)

	epochs, stats, err := Parse(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if stats.TotalMessages != 1 {
		t.Errorf("TotalMessages = %d, want 1", stats.TotalMessages)
	}
	if stats.RawxMessages != 0 {
		t.Errorf("RawxMessages = %d, want 0", stats.RawxMessages)
	}
	if len(epochs) != 0 {
		t.Errorf("expected 0 epochs for non-RAWX message, got %d", len(epochs))
	}
}

func TestMultipleEpochs(t *testing.T) {
	var input []byte
	for i := 0; i < 5; i++ {
		payload := buildRawxPayload(float64(100000+i*30), 2200, 18, 0x01, []RawxMeas{
			{PrMes: 22000000.0, GnssID: 0, SvID: uint8(i + 1), TrkStat: 0x03},
		})
		input = append(input, buildUBXFrame(ClassRXM, IDRawx, payload)...)
	}

	epochs, stats, err := Parse(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(epochs) != 5 {
		t.Fatalf("expected 5 epochs, got %d", len(epochs))
	}
	if stats.TotalMessages != 5 {
		t.Errorf("TotalMessages = %d, want 5", stats.TotalMessages)
	}
	if stats.RawxMessages != 5 {
		t.Errorf("RawxMessages = %d, want 5", stats.RawxMessages)
	}
}

func TestLockTimeDecoding(t *testing.T) {
	payload := buildRawxPayload(100000.0, 2200, 18, 0x01, []RawxMeas{
		{GnssID: 0, SvID: 1, Locktime: 5000, TrkStat: 0x03},
	})
	frame := buildUBXFrame(ClassRXM, IDRawx, payload)

	epochs, _, err := Parse(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(epochs) != 1 || len(epochs[0].Satellites) != 1 || len(epochs[0].Satellites[0].Signals) != 1 {
		t.Fatal("unexpected epoch structure")
	}

	lockSec := epochs[0].Satellites[0].Signals[0].LockTimeSec
	expected := 5.0 // 5000 / 1000.0
	if math.Abs(lockSec-expected) > 1e-9 {
		t.Errorf("LockTimeSec = %f, want %f", lockSec, expected)
	}
}

func TestConstellationMapping(t *testing.T) {
	tests := []struct {
		gnssID uint8
		want   gnss.Constellation
		satID  string
	}{
		{0, gnss.ConsGPS, "G01"},
		{1, gnss.ConsSBAS, "S01"},
		{2, gnss.ConsGalileo, "E01"},
		{3, gnss.ConsBeiDou, "C01"},
		{5, gnss.ConsQZSS, "J01"},
		{6, gnss.ConsGLONASS, "R01"},
	}

	for _, tc := range tests {
		payload := buildRawxPayload(100000.0, 2200, 18, 0x01, []RawxMeas{
			{GnssID: tc.gnssID, SvID: 1, TrkStat: 0x01},
		})
		frame := buildUBXFrame(ClassRXM, IDRawx, payload)
		epochs, _, err := Parse(bytes.NewReader(frame))
		if err != nil {
			t.Fatalf("gnssID %d: Parse error: %v", tc.gnssID, err)
		}
		if len(epochs) != 1 || len(epochs[0].Satellites) != 1 {
			t.Fatalf("gnssID %d: unexpected structure", tc.gnssID)
		}
		sat := epochs[0].Satellites[0]
		if sat.Constellation != tc.want {
			t.Errorf("gnssID %d: constellation = %d, want %d", tc.gnssID, sat.Constellation, tc.want)
		}
		if sat.SatID() != tc.satID {
			t.Errorf("gnssID %d: SatID = %q, want %q", tc.gnssID, sat.SatID(), tc.satID)
		}
	}
}

func TestEmptyInput(t *testing.T) {
	epochs, stats, err := Parse(bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("Parse error on empty input: %v", err)
	}
	if len(epochs) != 0 {
		t.Errorf("expected 0 epochs, got %d", len(epochs))
	}
	if stats.TotalMessages != 0 {
		t.Errorf("expected 0 total messages, got %d", stats.TotalMessages)
	}
}

func TestDopplerSign(t *testing.T) {
	payload := buildRawxPayload(100000.0, 2200, 18, 0x01, []RawxMeas{
		{DoMes: -3456.789, GnssID: 0, SvID: 1, TrkStat: 0x01},
	})
	frame := buildUBXFrame(ClassRXM, IDRawx, payload)
	epochs, _, err := Parse(bytes.NewReader(frame))
	if err != nil {
		t.Fatal(err)
	}
	got := epochs[0].Satellites[0].Signals[0].Doppler
	if math.Abs(got-float64(float32(-3456.789))) > 0.01 {
		t.Errorf("Doppler = %f, want ~-3456.789", got)
	}
}
