package ubx

// Checksum computes the UBX Fletcher-8 checksum over data.
// data should include class + id + length + payload (everything between
// the sync bytes and the checksum itself).
func Checksum(data []byte) (ckA, ckB uint8) {
	for _, b := range data {
		ckA += b
		ckB += ckA
	}
	return ckA, ckB
}

// ValidChecksum returns true if the Fletcher-8 checksum of data matches ckA/ckB.
func ValidChecksum(data []byte, ckA, ckB uint8) bool {
	a, b := Checksum(data)
	return a == ckA && b == ckB
}

// ChecksumParts computes Fletcher-8 over multiple byte slices without concatenation.
func ChecksumParts(parts ...[]byte) (ck_a, ck_b uint8) {
	var a, b uint8
	for _, part := range parts {
		for _, v := range part {
			a += v
			b += a
		}
	}
	return a, b
}
