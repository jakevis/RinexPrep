# Copilot Instructions — RinexPrep

## Build, Test, Lint

```bash
make build                # → bin/rinexprep
make test                 # go test -race -coverprofile ./...
make test-short           # skip -race flag
make lint                 # golangci-lint run ./...
make fmt                  # go fmt + goimports

# Run a single test
go test -v -run TestHalfCycleExcluded ./internal/rinex/

# Run tests for one package
go test -v ./internal/pipeline/

# Frontend (web/)
cd web && npm ci && npm run build   # production build → web/dist/
cd web && npm run dev               # Vite dev server with hot reload

# Run the server locally
go run ./cmd/rinexprep serve --port 8080 --data-dir ./data

# Docker
make docker-build && make docker-run
```

## Architecture

RinexPrep converts raw u-blox UBX binary GNSS data into OPUS-compatible RINEX observation files. No external tools (RTKLIB, convbin, gfzrnx) — all parsing and generation is native Go.

### Data Flow

```
UBX binary → ubx.Parse() → []gnss.Epoch → pipeline.Process() → rinex.WriteRinex{2,3}()
                                                ↓
                                          qc.Analyze() → OPUS readiness report
```

### Package Responsibilities

| Package | Purpose |
|---------|---------|
| `internal/ubx` | Stream-based UBX binary parser (RXM-RAWX, RXM-SFRBX, NAV-SAT) |
| `internal/gnss` | Core data model: `GNSSTime`, `Epoch`, `SatObs`, `Signal`, `Metadata` |
| `internal/pipeline` | Processing: AutoTrim → Trim → Filter → Normalize (30s grid snap) |
| `internal/rinex` | RINEX 2.11 and 3.03 writers with UBX→RINEX signal mapping |
| `internal/qc` | Quality metrics and OPUS readiness scoring |
| `internal/api` | REST API with async job model, serves embedded React SPA |
| `cmd/rinexprep` | CLI entry: `convert`, `serve`, `version` subcommands |
| `frontend/` | Go embed wrapper for the React SPA built from `web/` |
| `web/` | React 19 + TypeScript + Vite + Tailwind CSS frontend |

### Pipeline Stages (in order)

1. **AutoTrim** — removes startup/teardown instability (min 5 GPS sats, avg SNR ≥ 30 dB-Hz, 10 consecutive stable epochs)
2. **Trim** — optional manual time window
3. **Filter** — GPS-only (default), drop epochs with < 4 satellites
4. **Normalize** — snap to 30s grid (±100ms tolerance), deduplicate, sort

### API Endpoints

```
POST   /api/v1/upload              Multipart UBX upload, creates job
GET    /api/v1/jobs/{id}/status    Poll job status
GET    /api/v1/jobs/{id}/preview   Satellite visibility, QC preview
POST   /api/v1/jobs/{id}/trim      Set manual trim bounds
POST   /api/v1/jobs/{id}/process   Generate RINEX output
GET    /api/v1/jobs/{id}/download  Download result ZIP
POST   /api/v1/jobs/{id}/opus      Submit to NGS OPUS
DELETE /api/v1/jobs/{id}           Delete job
```

Job lifecycle: `uploaded → parsing → preview → processing → ready` (or `failed`).

## Key Conventions

### GNSS Time

Use `gnss.GNSSTime` (GPS week + nanosecond TOW), **never** `time.Time` for internal computation. `time.Time` conversion happens only at output boundaries (RINEX writer, API JSON). Nanosecond TOW avoids float64 drift that corrupts grid snapping. Grid arithmetic operates directly on `TOWNanos`.

### Signal Model — Late RINEX Mapping

The internal model stores **raw UBX signal identifiers** (`GnssID`, `SigID`, `FreqBand`). RINEX observation codes (C1C, L2X, etc.) are assigned at write time in the `rinex` package, not in the data model. This keeps the core model RINEX-version-agnostic.

### RINEX 2.11: C2, Never P2

The u-blox F9P tracks L2C (civilian), not P-code. RINEX 2.11 output **must** use `C2`, not `P2`. Labeling L2C as P2 causes OPUS rejection. This is enforced in `rinex/writer2.go`.

### Carrier Phase Quality

When the UBX half-cycle ambiguity flag is set (`HalfCycle=true && SubHalfCyc=false`), carrier phase observations are **suppressed** (not output). Pseudorange, Doppler, and SNR are unaffected. When `SubHalfCyc=true`, the receiver has corrected the ambiguity and phase is safe to output.

Both RINEX writers maintain **stateful LLI (loss-of-lock indicator) tracking** per satellite per frequency band across epochs. LLI=1 is set when:
- Lock time = 0 (explicit receiver cycle slip)
- Lock time decreases vs previous epoch (implicit cycle slip between observation intervals)
- Carrier phase resumes after a gap or half-cycle suppression (new phase arc)

### Missing Observations

Write **blank fields** (spaces), never `0.000`. OPUS parsers treat zero as a real observation value.

### Signal Selection

When multiple signals exist for the same frequency band (e.g., GPS L2 CL and L2 CM), `bestSignalForBand()` selects the one with the lowest `SigID` (most reliable).

### Error Handling

- UBX parser: corrupt checksums are skipped with re-sync, truncated files complete with available data
- Pipeline: empty inputs return nil gracefully, off-grid epochs are dropped (no interpolation)
- Metadata: `Validate()` returns warnings, not errors — processing continues with placeholder values
- CLI: errors to stderr, exit code 1; warnings logged but don't halt processing

### Logging

Uses `log/slog`. JSON mode via `--json-logs` flag on `serve` command. No third-party logging libraries.

### Test Patterns

- Table-driven tests with `*_test.go` alongside implementation
- Test helpers: `makeGPSSat()`, `makeEpoch()`, `makeSatObs()`, `testMetadata()` for building GNSS fixtures
- Integration test fixture: `testdata/fixtures/sample_30s.ubx` (real UBX data)
- RINEX writer tests validate exact character positions for field alignment and LLI/SS flags

## Domain Reference

### OPUS Readiness Thresholds (from `qc/opus.go`)

| Metric | Failure | Warning |
|--------|---------|---------|
| Duration | < 15 min | < 2 hours |
| GPS sats (mean) | < 4 | 4–6 |
| L2 coverage | < 10% | < 80% |
| Max gap | > 600s | > 120s |
| Obs completeness | — | < 80% |
| Cycle slips | — | > 50 |

### Pipeline Defaults

| Setting | Default | Source |
|---------|---------|--------|
| Grid interval | 30s | `pipeline.DefaultNormalizeConfig()` |
| Snap tolerance | 100ms | `pipeline.DefaultNormalizeConfig()` |
| Constellation | GPS-only | `pipeline.DefaultFilterConfig()` |
| Min satellites/epoch | 4 | `pipeline.DefaultFilterConfig()` |
| AutoTrim min sats | 5 | `pipeline.DefaultAutoTrimConfig()` |
| AutoTrim min SNR | 30.0 dB-Hz | `pipeline.DefaultAutoTrimConfig()` |
| AutoTrim stability window | 10 epochs | `pipeline.DefaultAutoTrimConfig()` |

### UBX Tracking Status Bits (`trkStat` byte in RXM-RAWX)

| Bit | Flag | When SET (1) | When CLEAR (0) |
|-----|------|-------------|----------------|
| 0 | `PRValid` | Pseudorange valid | Invalid |
| 1 | `CPValid` | Carrier phase valid | Invalid |
| 2 | `halfCyc` | Half-cycle **resolved** (good) | Half-cycle **unresolved** (ambiguous) |
| 3 | `SubHalfCyc` | Half-cycle subtracted by receiver | Not subtracted |

**Critical polarity note:** UBX bit 2 SET means the data is GOOD (resolved). The `gnss.Signal.HalfCycle` field **inverts** this: `HalfCycle=true` means the ambiguity is **unresolved** (bad). This inversion happens in `ubx/rawx.go` so downstream code can use `HalfCycle` intuitively as a "problem flag".
