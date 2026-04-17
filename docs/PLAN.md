# RinexPrep — Technical Design Plan

## Problem Statement

Build a production-grade system to ingest raw u-blox UBX binary GNSS data and produce OPUS-compatible RINEX observation files. All parsing and generation is implemented directly — no external tools (RTKLIB, convbin, gfzrnx).

## Language: Go

**Justification:**
- First-class binary parsing (`encoding/binary`, `io.Reader`, byte-level control)
- Single static binary → minimal Docker images (distroless)
- Strong typing catches GNSS domain errors at compile time
- Goroutines for pipeline concurrency
- No runtime dependencies, trivial cross-compilation

---

## 1. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         RinexPrep                               │
│                                                                 │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌─────────────┐ │
│  │  UBX     │──▶│  GNSS    │──▶│ Pipeline │──▶│ RINEX       │ │
│  │  Parser  │   │  Model   │   │ (norm/   │   │ Writer      │ │
│  │          │   │          │   │  filter) │   │ (2.11/3.x)  │ │
│  └──────────┘   └──────────┘   └──────────┘   └─────────────┘ │
│       │              │              │                │          │
│       │              ▼              ▼                │          │
│       │         ┌──────────┐  ┌──────────┐          │          │
│       │         │ Metadata │  │ QC       │          │          │
│       │         │ Layer    │  │ Engine   │──▶ report│          │
│       │         └──────────┘  └──────────┘          │          │
│       │                                             │          │
│  ┌────▼─────────────────────────────────────────────▼────────┐ │
│  │                    REST API / CLI                          │ │
│  └───────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Module Breakdown

| Package | Path | Responsibility |
|---------|------|----------------|
| `ubx` | `internal/ubx/` | Stream-based UBX binary parser |
| `gnss` | `internal/gnss/` | Core data model, time handling, constellations |
| `pipeline` | `internal/pipeline/` | Epoch normalization, filtering, trimming |
| `rinex` | `internal/rinex/` | RINEX 2.11 and 3.x file writers |
| `qc` | `internal/qc/` | Quality control engine, OPUS readiness |
| `api` | `internal/api/` | REST API with async job model |
| `cmd/rinexprep` | `cmd/rinexprep/` | CLI entry point |

---

## 2. Data Model

### GNSSTime (GNSS-native, not time.Time)

```go
type GNSSTime struct {
    Week        uint16     // GPS week (continuous)
    TOWNanos    int64      // Time of week in nanoseconds
    TimeSystem  TimeSystem // GPS, GLONASS, Galileo, BeiDou
    LeapSeconds int8       // GPS-UTC offset
    LeapValid   bool       // Receiver confirmed leap count
    ClkReset    bool       // Clock reset at this epoch
}
```

**Key decisions:**
- Nanosecond TOW avoids float64 drift that would corrupt grid snapping
- Leap seconds come from UBX NAV-TIMEGPS / receiver, NOT hard-coded
- `time.Time` conversion happens only at output boundaries (RINEX writer, API)
- Grid arithmetic operates directly on `TOWNanos`

### Signal (raw observation, pre-RINEX mapping)

```go
type Signal struct {
    GnssID       uint8    // UBX gnssId
    SigID        uint8    // UBX sigId
    FreqBand     uint8    // L1=0, L2=1, L5=2
    Pseudorange  float64  // meters
    CarrierPhase float64  // cycles
    Doppler      float64  // Hz
    SNR          float64  // dB-Hz
    LockTimeSec  float64  // continuous lock seconds
    PRValid      bool
    CPValid      bool
    HalfCycle    bool     // unresolved half-cycle ambiguity
    SubHalfCyc   bool     // half-cycle subtracted by receiver
}
```

**RINEX code mapping is a formatting concern**, not a data model concern. The `rinex` package contains the mapping table:

```
UBX (gnssId=0, sigId=0) → GPS L1 C/A   → RINEX3: C1C, L1C, D1C, S1C → RINEX2: C1, L1, D1, S1
UBX (gnssId=0, sigId=3) → GPS L2 CL    → RINEX3: C2L, L2L, D2L, S2L → RINEX2: C2, L2, D2, S2
UBX (gnssId=0, sigId=4) → GPS L2 CM    → RINEX3: C2S, L2S, D2S, S2S → RINEX2: C2, L2, D2, S2
```

**Critical OPUS note:** RINEX 2.11 `P2` implies P-code, but F9P tracks L2C (civilian). We write `C2` not `P2`. OPUS handles this, but we must NOT label L2C as P2.

### Epoch, SatObs

```go
type Epoch struct {
    Time       GNSSTime
    Satellites []SatObs
    Flag       uint8      // 0=OK, 1=power failure
}

type SatObs struct {
    Constellation Constellation
    PRN           uint8
    Signals       []Signal
}
```

### Metadata

```go
type Metadata struct {
    MarkerName, MarkerNumber          string
    ReceiverNumber, ReceiverType, ReceiverVersion string
    AntennaNumber, AntennaType        string
    AntennaDeltaH, AntennaDeltaE, AntennaDeltaN float64
    ApproxX, ApproxY, ApproxZ         float64
    Observer, Agency                  string
    // Computed
    Interval    float64
    FirstEpoch  GNSSTime
    LastEpoch   GNSSTime
    ObsTypes    []string
    LeapSeconds int8
}
```

OPUS requires: antenna type, approximate position, receiver type. The metadata layer validates these are present before writing and warns on missing fields.

---

## 3. UBX Parsing Strategy

### Binary Protocol Structure

```
┌──────┬───────┬─────┬────────┬──────────┬──────────┐
│ 0xB5 │ 0x62  │Class│  ID    │ Length   │ Payload  │ CK_A │ CK_B │
│ sync │ sync  │ 1B  │  1B    │ 2B LE   │ N bytes  │  1B  │  1B  │
└──────┴───────┴─────┴────────┴──────────┴──────────┘
```

### Parser Algorithm

```
func Parse(r io.Reader) <-chan Message:
    for:
        1. Scan for sync bytes 0xB5, 0x62
           - On non-sync byte: skip, increment error counter
           - This handles corrupt data / partial writes
        2. Read class (1B) + id (1B) + length (2B little-endian)
        3. Read payload (length bytes)
        4. Read checksum (2B)
        5. Validate Fletcher-8 checksum over class+id+length+payload
           - On failure: log, re-sync from byte after failed sync
        6. Dispatch by class+id:
           - (0x02, 0x15) → decode RXM-RAWX → Epoch
           - (0x02, 0x13) → decode RXM-SFRBX → NavData
           - other → skip (but count for diagnostics)
        7. Emit decoded message to channel
```

### RXM-RAWX Decoding (class=0x02, id=0x15)

```
Header (16 bytes):
  rcvTow    float64  — receiver TOW in seconds
  week      uint16   — GPS week
  leapS     int8     — leap seconds
  numMeas   uint8    — number of measurements
  recStat   uint8    — bit 0: leapSec valid, bit 1: clkReset
  ...reserved...

Per-measurement block (32 bytes × numMeas):
  prMes     float64  — pseudorange (m)
  cpMes     float64  — carrier phase (cycles)
  doMes     float32  — doppler (Hz)
  gnssId    uint8
  svId      uint8
  sigId     uint8
  freqId    uint8
  locktime  uint16   — lock time (scaled)
  cno       uint8    — C/N0 (dB-Hz)
  prStDev   uint8    — pseudorange stdev index
  cpStDev   uint8    — carrier phase stdev index
  doStDev   uint8    — doppler stdev index
  trkStat   uint8    — bit 0: prValid, bit 1: cpValid, bit 2: halfCyc, bit 3: subHalfCyc
  ...reserved...
```

### Error Handling

- **Partial file:** parser completes with what it has, reports truncation
- **Corrupt checksums:** skip message, re-sync, count errors
- **Missing messages:** QC engine detects gaps from epoch timeline
- **Receiver reset:** detected via `clkReset` flag, epoch flagged

---

## 4. RINEX Generation

### RINEX 2.11 Format (OPUS Primary)

#### Header Template
```
     2.11           OBSERVATION DATA    G (GPS)             RINEX VERSION / TYPE
RinexPrep v0.1      [agency]            [date]              PGM / RUN BY / DATE
[marker]                                                    MARKER NAME
[marker#]                                                   MARKER NUMBER
[observer]          [agency]                                OBSERVER / AGENCY
[rec#]              [rectype]           [recver]            REC # / TYPE / VERS
[ant#]              [anttype]                               ANT # / TYPE
[X]  [Y]  [Z]                                              APPROX POSITION XYZ
[dH]  [dE]  [dN]                                           ANTENNA: DELTA H/E/N
     1     1                                                WAVELENGTH FACT L1/2
     8    C1    P2    L1    L2    D1    D2    S1    S2      # / TYPES OF OBSERV
    30.000                                                  INTERVAL
  [yr] [mo] [dy] [hr] [mn] [sc]     GPS                    TIME OF FIRST OBS
  [yr] [mo] [dy] [hr] [mn] [sc]     GPS                    TIME OF LAST OBS
                                                            END OF HEADER
```

#### Observation Records
```
 YY MM DD HH MM SS.SSSSSSS  0 NN[sat list]
[C1         ][P2         ][L1         ][L2         ][D1         ][D2         ]
[S1         ][S2         ]
```

Each observable: 14 chars wide (F14.3), plus 2 chars for LLI+signal strength flags.

#### UBX → RINEX 2.11 Obs Type Mapping

| UBX gnssId | UBX sigId | Signal | RINEX 2.11 | Notes |
|-----------|----------|--------|------------|-------|
| 0 (GPS) | 0 | L1 C/A | C1, L1, D1, S1 | Primary L1 |
| 0 (GPS) | 3 | L2 CL | C2*, L2, D2, S2 | *Write as C2, NOT P2 |
| 0 (GPS) | 4 | L2 CM | C2*, L2, D2, S2 | Prefer sigId=3 if both |

**Missing signals:** write blank (spaces), not zero. OPUS parsers treat 0.000 as a real observation.

### RINEX 3.x Format (Secondary)

- Uses 3-char obs codes (C1C, L1C, etc.)
- One satellite per line, system identifier prefix
- More signals representable without lossy collapse
- Generated alongside 2.11 when requested

---

## 5. Normalization Algorithm

### 30-Second Grid Snapping

```
Input: []Epoch (sorted by time)
Config:
  - gridInterval: 30s (fixed for OPUS)
  - snapTolerance: 100ms (configurable, default tight)
  - trimStart, trimEnd: optional time bounds

Algorithm:
  1. Sort epochs by GNSSTime (should already be sorted, verify)
  2. For each epoch:
     a. Compute offset = epoch.TOWNanos mod (30 * 1e9)
        Normalize to range [-15s, +15s]
     b. If |offset| ≤ snapTolerance:
        - Snap: epoch.TOWNanos -= offset
     c. Else:
        - Drop epoch (off-grid, no interpolation)
  3. Deduplicate: if multiple epochs snap to same grid point,
     keep the one with smallest |original offset| (closest to grid)
  4. Apply trim: drop epochs outside [trimStart, trimEnd]
  5. Verify monotonically increasing timestamps
```

**Why 100ms default (not ±0.5s):**
- F9P RAWX timestamps have sub-millisecond jitter
- ±0.5s is dangerously wide — could grab wrong epoch when decimating from 1Hz
- 100ms catches normal jitter without false matches
- Configurable for degraded receivers

### Decimation (e.g., 1Hz → 30s)

When input is higher-rate (1Hz, 5Hz):
1. Compute all 30s grid points in the session range
2. For each grid point, find the closest epoch within tolerance
3. Use that epoch's observations directly (no interpolation)

---

## 6. QC Engine / Validation Rules

### Metrics Computed

| Metric | Description | Method |
|--------|-------------|--------|
| `total_epochs` | Number of observation epochs | Count |
| `duration_hours` | Session length | Last - First epoch |
| `gps_sat_count` | GPS sats per epoch | Per-epoch count, min/max/mean |
| `l2_coverage_pct` | % of GPS sats with L2 signal | Signal presence check |
| `epoch_gaps` | Gaps > 2× interval | Sequential diff |
| `max_gap_seconds` | Largest gap | Max of diffs |
| `cycle_slip_count` | Estimated cycle slips | Lock time resets + LLI |
| `multipath_indicators` | SNR variance patterns | Per-sat SNR analysis |
| `observation_completeness` | % of expected epochs present | actual / expected |

### OPUS Readiness Assessment

```go
type OPUSReadiness struct {
    Ready    bool
    Score    float64  // 0-100
    Failures []string // blocking issues
    Warnings []string // non-blocking concerns
}
```

**Pass criteria:**
- Duration ≥ 2 hours (warn if < 4 hours)
- GPS satellites ≥ 4 per epoch (≥ 90% of epochs)
- L2 coverage ≥ 50% (warn if < 80%)
- Max gap < 600 seconds
- GPS-only constellation
- Static data (no kinematic movement detected)
- Required metadata present (antenna type, approx position)
- No half-cycle ambiguity on majority of observations

**Static detection:**
- NOT pseudorange variance (too noisy per rubber-duck review)
- Instead: carrier-phase consistency check — compute epoch-differenced carrier phase residuals per satellite. Static receivers show smooth residuals; kinematic show jumps.
- Fallback: user-declared via metadata/config

### Cycle Slip Detection (Heuristic)

- Lock time reset to zero → definite slip
- Large carrier-phase jump between epochs (> 5 cycles on L1) → probable slip
- Set LLI flag in RINEX output accordingly

---

## 7. API Design

### Single Async Job Model

```
POST   /api/v1/jobs            Create processing job (multipart: file + options)
GET    /api/v1/jobs/{id}       Job status + metadata
GET    /api/v1/jobs/{id}/qc    QC report (JSON)
GET    /api/v1/jobs/{id}/files Download results (zip: .obs, .nav, qc.json)
DELETE /api/v1/jobs/{id}       Cancel/delete job
```

### Create Job Request

```http
POST /api/v1/jobs
Content-Type: multipart/form-data

file: raw.ubx
options: {
  "format": "rinex2",        // rinex2 | rinex3 | both
  "interval": 30,            // seconds
  "constellation": "gps",    // gps | all
  "trim_start": "...",       // ISO 8601, optional
  "trim_end": "...",         // ISO 8601, optional
  "metadata": {
    "marker_name": "MYSITE",
    "antenna_type": "TRM57971.00     NONE",
    "antenna_height": 1.543,
    "receiver_type": "SPARKFUN FACET MOSAIC"
  }
}
```

### Job Status Response

```json
{
  "id": "job_abc123",
  "status": "completed",
  "created_at": "2026-04-16T17:00:00Z",
  "completed_at": "2026-04-16T17:00:12Z",
  "input_file": "raw.ubx",
  "input_size_bytes": 52428800,
  "output_files": ["session.obs", "qc.json"],
  "qc_summary": {
    "opus_ready": true,
    "score": 92,
    "duration_hours": 4.2,
    "gps_satellites_mean": 9.3,
    "l2_coverage_pct": 87.2
  }
}
```

### Security Considerations
- Max file upload: 500MB (configurable)
- Job retention: 24 hours then auto-delete
- Rate limiting per IP
- Input validation: UBX sync byte check before full parse
- No user auth — all data is transient, jobs auto-expire

---

## 8. Implementation Roadmap

### Phase 1 — MVP: CLI UBX→RINEX2 ✅

| Todo | Status | Description |
|------|--------|-------------|
| `ubx-parser` | ✅ | Stream parser with sync/checksum/RAWX decode |
| `gnss-time` | ✅ | GNSSTime model with grid snapping |
| `gnss-model` | ✅ | Epoch/SatObs/Signal data structures |
| `signal-map` | ✅ | UBX sigId → RINEX 2.11 obs code table |
| `rinex2-writer` | ✅ | RINEX 2.11 header + observation writer |
| `pipeline-normalize` | ✅ | 30s grid snap + GPS filter + dedup |
| `cli-convert` | ✅ | `rinexprep convert` command |
| `integration-test` | ✅ | End-to-end with known-good UBX fixture |

### Phase 2 — v1: QC + API ✅

| Todo | Status | Description |
|------|--------|-------------|
| `qc-engine` | ✅ | Compute all QC metrics |
| `opus-readiness` | ✅ | OPUS compatibility scoring |
| `metadata-layer` | ✅ | User-provided + UBX-extracted metadata |
| `api-server` | ✅ | REST API with job model |
| `rinex3-writer` | ✅ | RINEX 3.x writer |
| `cli-analyze` | — | Deferred; QC available via API preview endpoint |

### Phase 3 — v2: Production Hardening ✅

| Todo | Status | Description |
|------|--------|-------------|
| `sfrbx-parser` | ✅ | Navigation/ephemeris message decode |
| `rate-limiting` | ✅ | Per-IP rate limiting |
| `cycle-slip` | ✅ | Advanced cycle-slip detection (`pipeline/slipdetect.go`) |
| `clock-corr` | ✅ | RTKLIB-style receiver clock correction (`pipeline/clockcorr.go`) |
| `auto-trim` | ✅ | Startup/teardown instability removal (`pipeline/autotrim.go`) |
| `batch-processing` | ✅ | Multi-file upload support |
| `web-ui` | ✅ | React 19 + TypeScript + Vite + Tailwind CSS SPA |
| `opus-submit` | ✅ | Direct OPUS submission endpoint |
| `chunked-upload` | ✅ | Chunked file uploads for Cloudflare Tunnel compatibility |

**Descoped:**
- `auth-api` — No authentication; all data is transient with auto-expiring jobs
- `object-storage` — Local filesystem only; transient data does not warrant cloud storage

### Phase 4 — AUSPOS Integration (in progress, `feat/auspos-support` branch)

| Todo | Status | Description | Issue |
|------|--------|-------------|-------|
| `auspos-handler` | 🔲 | Backend submission handler (`api/auspos.go`) | #38 |
| `auspos-validation` | 🔲 | AUSPOS-specific RINEX validation warnings | #39 |
| `auspos-frontend` | 🔲 | Service selector UI (OPUS / AUSPOS) in DownloadPanel | #40 |
| `auspos-docs` | 🔲 | Documentation updates (README, PLAN, instructions) | #41 |

AUSPOS (Geoscience Australia) provides ITRF2020, GDA2020, and GDA94 coordinates.
Free, no login required, works globally. GPS-only L1+L2, same as OPUS.
See #37 for full requirements and plan.

---

## 9. Project Structure

```
RinexPrep/
├── .devcontainer/        # Dev container config
│   ├── devcontainer.json
│   └── Dockerfile
├── cmd/
│   └── rinexprep/
│       └── main.go       # CLI: convert, serve, version
├── internal/
│   ├── ubx/              # UBX binary parser
│   │   ├── parser.go     # Stream parser
│   │   ├── rawx.go       # RXM-RAWX decoder
│   │   ├── gpsephem.go   # GPS ephemeris decode
│   │   ├── navsat.go     # NAV-SAT decoder
│   │   ├── navpvt.go     # NAV-PVT decoder
│   │   ├── message.go    # Message types
│   │   └── checksum.go   # Fletcher-8
│   ├── gnss/             # Core data model
│   │   ├── time.go       # GNSSTime
│   │   ├── observation.go # Epoch, SatObs, Signal
│   │   └── metadata.go   # Session metadata
│   ├── pipeline/         # Preprocessing
│   │   ├── pipeline.go   # Orchestrator
│   │   ├── autotrim.go   # Startup/teardown removal
│   │   ├── trim.go       # Manual time window
│   │   ├── filter.go     # Constellation filter
│   │   ├── normalize.go  # Grid snapping
│   │   ├── clockcorr.go  # Receiver clock correction
│   │   ├── slipdetect.go # Cycle-slip detection
│   │   └── arcprune.go   # Arc pruning
│   ├── rinex/            # RINEX writers
│   │   ├── signalmap.go  # UBX→RINEX mapping table
│   │   ├── timeconv.go   # Time conversion helpers
│   │   ├── writer2.go    # RINEX 2.11
│   │   └── writer3.go    # RINEX 3.03
│   ├── qc/               # Quality control
│   │   ├── engine.go     # QC computation
│   │   └── opus.go       # OPUS readiness
│   └── api/              # REST API
│       ├── server.go     # HTTP server + routing
│       ├── handlers.go   # Upload, process, download
│       ├── jobs.go       # Job lifecycle
│       ├── opus.go       # OPUS submission
│       ├── ratelimit.go  # Per-IP rate limiting
│       ├── logging.go    # Request logging
│       └── web.go        # Embedded SPA serving
├── frontend/             # Go embed wrapper for SPA
├── web/                  # React 19 + TypeScript + Vite + Tailwind
├── testdata/
│   └── fixtures/         # Test UBX data
├── docs/
│   └── PLAN.md           # This file
├── Dockerfile
├── Makefile
├── go.mod
└── README.md
```

---

## Key Design Decisions Log

1. **Go over Python** — binary parsing performance, single-binary deploy, strong typing
2. **GNSSTime over time.Time** — avoids leap-second bugs, enables integer grid arithmetic
3. **Late RINEX mapping** — internal model stores raw UBX signal IDs, RINEX codes assigned at write time
4. **100ms snap tolerance** — tighter than naive ±0.5s, matches real F9P jitter characteristics
5. **C2 not P2** — F9P tracks L2C (civilian), labeling as P2 causes OPUS failures
6. **Metadata layer** — validates OPUS-required fields before writing, fails early
7. **Single job API** — no multi-step upload/convert dance, reduces state bugs
8. **Carrier-phase static detection** — pseudorange variance is too noisy per expert review
9. **No auth gate** — all data is transient with auto-expiring jobs; no user accounts needed
10. **No object storage** — local filesystem sufficient for transient processing workloads
