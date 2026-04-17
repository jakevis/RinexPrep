package ubx

import (
	"encoding/binary"
	"math"
)

// NavPVT holds the position/velocity/time solution from a UBX NAV-PVT message.
type NavPVT struct {
	Lat    float64 // latitude in degrees
	Lon    float64 // longitude in degrees
	Height float64 // height above ellipsoid in meters
	HAcc   float64 // horizontal accuracy estimate in meters
	VAcc   float64 // vertical accuracy estimate in meters
	FixType uint8  // 0=no, 2=2D, 3=3D
	NumSV   uint8  // number of satellites used
}

// ECEF returns the approximate ECEF coordinates (X, Y, Z) in meters.
func (p *NavPVT) ECEF() (x, y, z float64) {
	const a = 6378137.0          // WGS84 semi-major axis
	const f = 1.0 / 298.257223563
	const e2 = 2*f - f*f

	latRad := p.Lat * math.Pi / 180.0
	lonRad := p.Lon * math.Pi / 180.0
	sinLat := math.Sin(latRad)
	cosLat := math.Cos(latRad)
	sinLon := math.Sin(lonRad)
	cosLon := math.Cos(lonRad)

	N := a / math.Sqrt(1-e2*sinLat*sinLat)
	x = (N + p.Height) * cosLat * cosLon
	y = (N + p.Height) * cosLat * sinLon
	z = (N*(1-e2) + p.Height) * sinLat
	return
}

// decodeNavPVT decodes a UBX NAV-PVT payload (92 bytes).
func decodeNavPVT(payload []byte) (*NavPVT, error) {
	if len(payload) < 92 {
		return nil, nil
	}

	fixType := payload[20]
	flags := payload[21]
	gnssFixOK := flags&0x01 != 0

	if !gnssFixOK || fixType < 2 {
		return nil, nil // no valid fix
	}

	return &NavPVT{
		Lon:     float64(int32(binary.LittleEndian.Uint32(payload[24:28]))) * 1e-7,
		Lat:     float64(int32(binary.LittleEndian.Uint32(payload[28:32]))) * 1e-7,
		Height:  float64(int32(binary.LittleEndian.Uint32(payload[32:36]))) * 1e-3,
		HAcc:    float64(binary.LittleEndian.Uint32(payload[40:44])) * 1e-3,
		VAcc:    float64(binary.LittleEndian.Uint32(payload[44:48])) * 1e-3,
		FixType: fixType,
		NumSV:   payload[23],
	}, nil
}
