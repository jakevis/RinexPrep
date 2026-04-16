package gnss

// Metadata holds session-level information required for RINEX headers
// and OPUS compatibility validation.
type Metadata struct {
	// Station / marker
	MarkerName   string
	MarkerNumber string

	// Receiver
	ReceiverNumber  string
	ReceiverType    string
	ReceiverVersion string

	// Antenna
	AntennaNumber string
	AntennaType   string
	AntennaDeltaH float64 // height above marker in meters
	AntennaDeltaE float64 // east offset
	AntennaDeltaN float64 // north offset

	// Approximate position (ECEF XYZ in meters)
	ApproxX float64
	ApproxY float64
	ApproxZ float64

	// Observer
	Observer string
	Agency   string

	// Computed from data
	Interval        float64  // observation interval in seconds
	FirstEpoch      GNSSTime // time of first observation
	LastEpoch       GNSSTime // time of last observation
	ObsTypes        []string // RINEX observation type codes present
	LeapSeconds     int8     // applied leap second count
}

// Validate checks that all OPUS-required metadata fields are populated.
// Returns a list of missing/invalid fields.
func (m Metadata) Validate() []string {
	var missing []string

	if m.AntennaType == "" {
		missing = append(missing, "antenna type (required by OPUS)")
	}
	if m.ApproxX == 0 && m.ApproxY == 0 && m.ApproxZ == 0 {
		missing = append(missing, "approximate position XYZ")
	}
	if m.ReceiverType == "" {
		missing = append(missing, "receiver type")
	}
	if m.AntennaDeltaH == 0 {
		// Warning, not error — could be ground mount
		// but OPUS strongly prefers this
	}

	return missing
}
