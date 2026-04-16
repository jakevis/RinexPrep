package ubx

import (
	"encoding/binary"
	"fmt"
)

// SfrbxMessage represents a decoded RXM-SFRBX message.
type SfrbxMessage struct {
	GnssID   uint8    // GNSS system identifier
	SvID     uint8    // Satellite ID
	FreqID   uint8    // Only used for GLONASS
	NumWords uint8    // Number of data words
	Channel  uint8    // Tracking channel
	Version  uint8    // Message version
	Words    []uint32 // Navigation data words
}

const sfrbxHeaderSize = 8

// decodeSfrbx decodes an RXM-SFRBX payload.
func decodeSfrbx(payload []byte) (*SfrbxMessage, error) {
	if len(payload) < sfrbxHeaderSize {
		return nil, fmt.Errorf("sfrbx payload too short: %d", len(payload))
	}

	msg := &SfrbxMessage{
		GnssID:   payload[0],
		SvID:     payload[1],
		FreqID:   payload[2],
		NumWords: payload[3],
		Channel:  payload[4],
		Version:  payload[5],
	}

	expectedLen := sfrbxHeaderSize + int(msg.NumWords)*4
	if len(payload) < expectedLen {
		return nil, fmt.Errorf("sfrbx payload too short for %d words: %d < %d",
			msg.NumWords, len(payload), expectedLen)
	}

	msg.Words = make([]uint32, msg.NumWords)
	for i := 0; i < int(msg.NumWords); i++ {
		off := sfrbxHeaderSize + i*4
		msg.Words[i] = binary.LittleEndian.Uint32(payload[off : off+4])
	}

	return msg, nil
}
