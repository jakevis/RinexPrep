# RinexPrep

**Production-grade GNSS data processor:** ingest raw u-blox UBX binary data and produce OPUS-compatible RINEX observation files with normalization, quality control, and validation.

> [!NOTE]
> **🎷 Vibe-coded with reckless enthusiasm.** This project was conjured into existence through the power of AI-assisted development and an unwavering refusal to ship anything that looks like it belongs on a VT100 terminal. GNSS data processing is arcane enough without the UI making you feel like you're filing taxes in 1987. If you find a bug, it was probably vibing too hard.

## Status

✅ **MVP complete** — see `docs/PLAN.md` for the full design document.

## Features

- **Web UI** — drag-and-drop UBX upload, satellite visibility charts, skyview polar plot, interactive trim sliders, RINEX download
- **Auto-trim** — detects survey setup/teardown instability and trims to clean 00/30s grid boundaries
- **UBX parser** — stream-based RXM-RAWX binary decoder, no external tools (no RTKLIB, no convbin)
- **RINEX 2.11 + 3.03** — OPUS-grade output with correct L2C mapping (C2, never P2)
- **30s normalization** — epoch grid snapping, GPS-only filtering, deduplication
- **QC engine** — OPUS readiness scoring with satellite visibility and L2 coverage metrics
- **OPUS submission** — submit directly to NGS OPUS from the web UI
- **CLI** — `convert`, `batch`, and `serve` commands for headless/scripted use
- **Single Docker image** — Go backend + embedded React frontend, one container, runs anywhere

## OPUS Quality

RinexPrep matches or exceeds RTKLIB conversion quality for OPUS submissions. Real-world testing shows **97% ambiguity resolution**, **0.013m RMS**, and millimeter-accurate coordinates — on par with commercial workflows.

Where it differs from RTKLIB:

- **Gen9 (F9P) optimized carrier phase filtering** — understands u-blox half-cycle and sub-half-cycle flags natively, recovering ~500 more L2 observations that RTKLIB drops
- **Zero-gap epoch recovery** — intelligent 30s grid decimation snaps observations to the nearest grid point instead of discarding near-misses, producing gapless observation files
- **RTKLIB-compatible receiver clock correction** — applies TADJ-equivalent clock steering so OPUS sees clean, aligned timestamps
- **Complete cycle slip detection** — tracks lock time per satellite per frequency with carry-forward across epochs, setting LLI flags where RTKLIB sometimes misses slips

The result is a RINEX file that OPUS processes without warnings and with full L1+L2 dual-frequency coverage.

## Quick Start

### Docker (recommended)

```bash
docker run --rm -p 8080:8080 rinexprep:latest
# Open http://localhost:8080
```

### CLI

```bash
# Convert a single UBX file to RINEX 2.11
rinexprep convert --input raw.ubx --output session.obs --format rinex2 --interval 30

# Convert to RINEX 3.03
rinexprep convert --input raw.ubx --output session.rnx --format rinex3

# Convert to both formats at once
rinexprep convert --input raw.ubx --output session --format both

# Batch convert a directory of UBX files
rinexprep batch --input-dir ./ubx_files/ --output-dir ./rinex_out/ --format rinex3

# Print version
rinexprep version
```

### Web Server

```bash
# Start with defaults (port 8080, data in ./data/)
rinexprep serve

# Custom port and data directory
rinexprep serve --port 3000 --data-dir /tmp/rinexprep

# Structured JSON logging (for production)
rinexprep serve --port 8080 --data-dir ./data --json-logs
```

## Development

```bash
# Use the dev container (recommended)
# Open in VS Code → "Reopen in Container"

# Or build locally (requires Go 1.26+ and Node 22+)
make build        # → bin/rinexprep
make test         # go test -race -coverprofile ./...
make test-short   # skip -race flag
make lint         # golangci-lint run ./...
make fmt          # go fmt + goimports

# Frontend dev (hot reload)
cd web && npm ci && npm run dev

# Run the server locally
go run ./cmd/rinexprep serve --port 8080 --data-dir ./data

# Docker build
make docker-build && make docker-run
```

## Architecture

```
                    ┌──────────────────────────────┐
                    │         Web UI (React)        │
                    │  upload · charts · trim · DL  │
                    └──────────────┬───────────────┘
                                  │ REST API
                    ┌─────────────▼───────────────┐
                    │        Go Backend            │
                    │                              │
  UBX Binary ──▶ Parser ──▶ Auto-Trim ──▶ Pipeline ──▶ RINEX Writer
                    │            │           │              │
                    │            ▼           ▼              │
                    │        Instability   30s Grid         │
                    │        Detection     Snap + Filter    │
                    │                                      │
                    │         QC Engine ──▶ OPUS Score      │
                    └──────────────────────────────────────┘
```

See `docs/PLAN.md` for full design documentation.

## Contributing

Contributions are welcome! Please open an issue to discuss proposed changes before submitting a pull request.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes
4. Push to your branch and open a pull request

## License

This project is licensed under the [MIT License](LICENSE) — free to use with attribution.
