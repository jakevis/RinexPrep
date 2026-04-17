package ubx

// UBX protocol constants.
const (
	SyncByte1 byte = 0xB5
	SyncByte2 byte = 0x62

	// RXM-RAWX message class and ID.
	ClassRXM  byte = 0x02
	IDRawx    byte = 0x15

	// RXM-SFRBX message ID (class is ClassRXM).
	IDSfrbx   byte = 0x13

	// NAV-SAT message class and ID.
	ClassNAV  byte = 0x01
	IDNavSat  byte = 0x35
	IDNavPVT  byte = 0x07

	// Header sizes.
	rawxHeaderSize = 16
	rawxMeasSize   = 32
)

// Message is a raw, parsed UBX frame before higher-level decoding.
type Message struct {
	Class   byte
	ID      byte
	Payload []byte
}

// RawxMeas holds one decoded per-satellite measurement block from RXM-RAWX.
type RawxMeas struct {
	PrMes    float64
	CpMes    float64
	DoMes    float32
	GnssID   uint8
	SvID     uint8
	SigID    uint8
	FreqID   uint8
	Locktime uint16
	Cno      uint8
	PrStDev  uint8
	CpStDev  uint8
	DoStDev  uint8
	TrkStat  uint8
}

// RawxEpoch holds the decoded header and measurement blocks of an RXM-RAWX message.
type RawxEpoch struct {
	RcvTow  float64
	Week    uint16
	LeapS   int8
	NumMeas uint8
	RecStat uint8
	Meas    []RawxMeas
}
