package ubx

import (
	"encoding/binary"
	"fmt"
)

// NavSatInfo holds satellite status from a NAV-SAT message.
type NavSatInfo struct {
	GnssID    uint8
	SvID      uint8
	Azimuth   int16  // degrees (0-360)
	Elevation int8   // degrees (0-90)
	CNO       uint8  // signal strength dB-Hz
	Flags     uint32 // quality/health flags
}

// NavSatEpoch holds all satellite info from one NAV-SAT message.
type NavSatEpoch struct {
	ITOW       uint32 // GPS time of week in ms
	NumSvs     uint8
	Satellites []NavSatInfo
}

const (
	navSatHeaderSize = 8  // iTOW(4) + version(1) + numSvs(1) + reserved(2)
	navSatBlockSize  = 12 // per-satellite block
)

// decodeNavSat decodes a UBX NAV-SAT payload.
func decodeNavSat(payload []byte) (*NavSatEpoch, error) {
	if len(payload) < navSatHeaderSize {
		return nil, fmt.Errorf("nav-sat payload too short: %d", len(payload))
	}

	iTOW := binary.LittleEndian.Uint32(payload[0:4])
	numSvs := payload[5]

	expected := navSatHeaderSize + int(numSvs)*navSatBlockSize
	if len(payload) < expected {
		return nil, fmt.Errorf("nav-sat payload too short for %d svs", numSvs)
	}

	epoch := &NavSatEpoch{
		ITOW:   iTOW,
		NumSvs: numSvs,
	}

	for i := 0; i < int(numSvs); i++ {
		off := navSatHeaderSize + i*navSatBlockSize
		block := payload[off : off+navSatBlockSize]

		info := NavSatInfo{
			GnssID:    block[0],
			SvID:      block[1],
			CNO:       block[2],
			Elevation: int8(block[3]),
			Azimuth:   int16(binary.LittleEndian.Uint16(block[4:6])),
			Flags:     binary.LittleEndian.Uint32(block[8:12]),
		}
		epoch.Satellites = append(epoch.Satellites, info)
	}

	return epoch, nil
}
