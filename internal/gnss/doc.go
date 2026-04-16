// Package gnss defines the core GNSS data model for RinexPrep.
//
// Design principles:
//   - GNSS-native time representation (week + TOW), not time.Time
//   - Signal-level storage with late RINEX code mapping
//   - Constellation-agnostic internal model, filtered at pipeline stage
package gnss
