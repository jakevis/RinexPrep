package ubx

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// ParseStats tracks statistics from a UBX parsing session.
type ParseStats struct {
	TotalMessages  int
	RawxMessages   int
	NavSatMessages int
	SfrbxMessages  int
	SkippedBytes   int64
	ChecksumErrors int
	NavSatData     []*NavSatEpoch
	Ephemerides    map[uint8]*GPSEphemeris // keyed by PRN
}

// Parse reads a UBX binary stream from r and returns decoded GNSS epochs.
// It scans for sync bytes, validates checksums, and decodes RXM-RAWX messages.
// Non-RAWX messages are counted but otherwise skipped.
func Parse(r io.Reader) ([]*gnss.Epoch, *ParseStats, error) {
	br := bufio.NewReaderSize(r, 64*1024) // 64KB read buffer
	var epochs []*gnss.Epoch
	stats := &ParseStats{}

	buf := make([]byte, 1)
	header := make([]byte, 4)
	var frameBuf []byte

	for {
		// Scan for first sync byte (0xB5).
		if _, err := io.ReadFull(br, buf); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return epochs, stats, nil
			}
			return epochs, stats, err
		}
		if buf[0] != SyncByte1 {
			stats.SkippedBytes++
			continue
		}

		// Read second sync byte.
		if _, err := io.ReadFull(br, buf); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return epochs, stats, nil
			}
			return epochs, stats, err
		}
		if buf[0] != SyncByte2 {
			stats.SkippedBytes += 2 // the 0xB5 + this byte
			continue
		}

		// Read class + id + length (4 bytes).
		if _, err := io.ReadFull(br, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return epochs, stats, nil
			}
			return epochs, stats, err
		}

		class := header[0]
		id := header[1]
		payloadLen := binary.LittleEndian.Uint16(header[2:4])

		// Read payload + 2 checksum bytes, reusing buffer.
		frameSize := int(payloadLen) + 2
		if cap(frameBuf) < frameSize {
			frameBuf = make([]byte, frameSize)
		} else {
			frameBuf = frameBuf[:frameSize]
		}
		if _, err := io.ReadFull(br, frameBuf); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return epochs, stats, nil
			}
			return epochs, stats, err
		}

		payload := frameBuf[:payloadLen]
		ckA := frameBuf[payloadLen]
		ckB := frameBuf[payloadLen+1]

		// Compute checksum over header + payload without allocating.
		ckAExpected, ckBExpected := ChecksumParts(header, payload)
		if ckAExpected != ckA || ckBExpected != ckB {
			stats.ChecksumErrors++
			continue
		}

		stats.TotalMessages++

		if class == ClassRXM && id == IDRawx {
			stats.RawxMessages++
			epoch, err := decodeRawx(payload)
			if err != nil {
				continue
			}
			epochs = append(epochs, epoch)
		}

		if class == ClassNAV && id == IDNavSat {
			stats.NavSatMessages++
			navSat, err := decodeNavSat(payload)
			if err == nil {
				stats.NavSatData = append(stats.NavSatData, navSat)
			}
		}

		if class == ClassRXM && id == IDSfrbx {
			stats.SfrbxMessages++
			sfrbx, err := decodeSfrbx(payload)
			if err == nil && sfrbx.GnssID == 0 { // GPS only
				if stats.Ephemerides == nil {
					stats.Ephemerides = make(map[uint8]*GPSEphemeris)
				}
				prn := sfrbx.SvID
				ephem, exists := stats.Ephemerides[prn]
				if !exists {
					ephem = &GPSEphemeris{PRN: prn}
					stats.Ephemerides[prn] = ephem
				}
				ParseGPSSubframe(sfrbx, ephem)
			}
		}
	}
}
