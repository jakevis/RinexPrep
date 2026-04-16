package ubx

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/jakevis/rinexprep/internal/gnss"
)

// ParseStats tracks statistics from a UBX parsing session.
type ParseStats struct {
	TotalMessages  int
	RawxMessages   int
	SkippedBytes   int64
	ChecksumErrors int
}

// Parse reads a UBX binary stream from r and returns decoded GNSS epochs.
// It scans for sync bytes, validates checksums, and decodes RXM-RAWX messages.
// Non-RAWX messages are counted but otherwise skipped.
func Parse(r io.Reader) ([]*gnss.Epoch, *ParseStats, error) {
	var epochs []*gnss.Epoch
	stats := &ParseStats{}

	buf := make([]byte, 1)

	for {
		// Scan for first sync byte (0xB5).
		if _, err := io.ReadFull(r, buf); err != nil {
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
		if _, err := io.ReadFull(r, buf); err != nil {
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
		header := make([]byte, 4)
		if _, err := io.ReadFull(r, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return epochs, stats, nil
			}
			return epochs, stats, err
		}

		class := header[0]
		id := header[1]
		payloadLen := binary.LittleEndian.Uint16(header[2:4])

		// Read payload + 2 checksum bytes.
		frame := make([]byte, int(payloadLen)+2)
		if _, err := io.ReadFull(r, frame); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return epochs, stats, nil
			}
			return epochs, stats, err
		}

		payload := frame[:payloadLen]
		ckA := frame[payloadLen]
		ckB := frame[payloadLen+1]

		// Checksummed data is class + id + length + payload.
		checksumData := make([]byte, 4+int(payloadLen))
		copy(checksumData, header)
		copy(checksumData[4:], payload)

		if !ValidChecksum(checksumData, ckA, ckB) {
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
	}
}
