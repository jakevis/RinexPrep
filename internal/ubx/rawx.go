package ubx

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// sigIDToFreqBand maps UBX sigId to frequency band for each constellation.
func sigIDToFreqBand(gnssID, sigID uint8) uint8 {
	switch gnssID {
	case 0: // GPS
		switch sigID {
		case 0, 5, 6:
			return 0 // L1
		case 3, 4:
			return 1 // L2
		}
	case 2: // Galileo
		switch sigID {
		case 0, 1:
			return 0 // E1
		case 5, 6:
			return 2 // E5b
		}
	case 3: // BeiDou
		switch sigID {
		case 0, 1:
			return 0 // B1I
		case 2, 3:
			return 1 // B2I
		}
	case 6: // GLONASS
		switch sigID {
		case 0:
			return 0 // L1
		case 2:
			return 1 // L2
		}
	}
	return 0
}

// decodeRawx decodes an RXM-RAWX payload into a gnss.Epoch.
func decodeRawx(payload []byte) (*gnss.Epoch, error) {
	if len(payload) < rawxHeaderSize {
		return nil, fmt.Errorf("rawx payload too short: %d < %d", len(payload), rawxHeaderSize)
	}

	rcvTow := math.Float64frombits(binary.LittleEndian.Uint64(payload[0:8]))
	week := binary.LittleEndian.Uint16(payload[8:10])
	leapS := int8(payload[10])
	numMeas := payload[11]
	recStat := payload[12]

	expectedLen := rawxHeaderSize + int(numMeas)*rawxMeasSize
	if len(payload) < expectedLen {
		return nil, fmt.Errorf("rawx payload too short for %d measurements: %d < %d",
			numMeas, len(payload), expectedLen)
	}

	leapValid := recStat&0x01 != 0
	clkReset := recStat&0x02 != 0

	towNanos := int64(rcvTow * 1e9)

	epoch := &gnss.Epoch{
		Time: gnss.GNSSTime{
			Week:        week,
			TOWNanos:    towNanos,
			TimeSystem:  gnss.TimeGPS,
			LeapSeconds: leapS,
			LeapValid:   leapValid,
			ClkReset:    clkReset,
		},
		Flag: 0,
	}

	// Group measurements by (gnssId, svId) into SatObs entries.
	type satKey struct {
		gnssID uint8
		svID   uint8
	}
	satMap := make(map[satKey]*gnss.SatObs)
	var satOrder []satKey

	for i := 0; i < int(numMeas); i++ {
		off := rawxHeaderSize + i*rawxMeasSize
		block := payload[off : off+rawxMeasSize]

		prMes := math.Float64frombits(binary.LittleEndian.Uint64(block[0:8]))
		cpMes := math.Float64frombits(binary.LittleEndian.Uint64(block[8:16]))
		doMes := math.Float32frombits(binary.LittleEndian.Uint32(block[16:20]))
		gnssID := block[20]
		svID := block[21]
		sigID := block[22]
		freqID := block[23]
		locktime := binary.LittleEndian.Uint16(block[24:26])
		cno := block[26]
		prStdev := block[27]
		cpStdev := block[28]
		doStdev := block[29]
		trkStat := block[30]

		_ = freqID
		_ = prStdev
		_ = doStdev

		// RTKLIB: invalidate carrier phase if cpStdev > 5 or value is -0.5
		cpValid := trkStat&0x02 != 0
		if cpStdev&0x0F > 5 || cpMes == -0.5 {
			cpValid = false
			cpMes = 0.0
		}

		sig := gnss.Signal{
			GnssID:       gnssID,
			SigID:        sigID,
			FreqBand:     sigIDToFreqBand(gnssID, sigID),
			Pseudorange:  prMes,
			CarrierPhase: cpMes,
			Doppler:      float64(doMes),
			SNR:          float64(cno),
			LockTimeSec:  float64(locktime) / 1000.0,
			PRValid:      trkStat&0x01 != 0,
			CPValid:      cpValid,
			HalfCycle:    trkStat&0x04 == 0, // bit 2 SET = resolved; we store true when UNresolved
			SubHalfCyc:   trkStat&0x08 != 0,
		}

		key := satKey{gnssID: gnssID, svID: svID}
		obs, exists := satMap[key]
		if !exists {
			obs = &gnss.SatObs{
				Constellation: gnss.Constellation(gnssID),
				PRN:           svID,
			}
			satMap[key] = obs
			satOrder = append(satOrder, key)
		}
		obs.Signals = append(obs.Signals, sig)
	}

	for _, key := range satOrder {
		epoch.Satellites = append(epoch.Satellites, *satMap[key])
	}

	return epoch, nil
}
