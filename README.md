# RinexPrep

**Production-grade GNSS data processor:** ingest raw u-blox UBX binary data and produce OPUS-compatible RINEX observation files with normalization, quality control, and validation.

## Status

🚧 **Under active development** — see `docs/PLAN.md` for the roadmap.

## Features (Planned)

- Stream-based UBX binary parser (RXM-RAWX, RXM-SFRBX)
- GNSS-native time model (GPS week + TOW nanoseconds)
- RINEX 2.11 output (OPUS primary target) and RINEX 3.x
- 30-second epoch normalization with grid snapping
- GPS-only constellation filtering for OPUS
- Quality control engine with OPUS readiness scoring
- REST API for job-based processing
- CLI for local conversion

## Quick Start

```bash
# CLI
rinexprep convert --input raw.ubx --output session.obs --format rinex2 --interval 30

# API
rinexprep serve --port 8080
```

## Development

```bash
# Use the dev container (recommended)
# Open in VS Code → "Reopen in Container"

# Or build locally (requires Go 1.22+)
make build
make test
make lint
```

## Architecture

```
UBX Binary → Parser → GNSS Model → Pipeline → RINEX Writer
                                       ↓
                                    QC Engine → Report
```

See `docs/PLAN.md` for full design documentation.

## License

TBD
