package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jakevis/rinexprep/frontend"
	"github.com/jakevis/rinexprep/internal/api"
	"github.com/jakevis/rinexprep/internal/gnss"
	"github.com/jakevis/rinexprep/internal/pipeline"
	"github.com/jakevis/rinexprep/internal/rinex"
	"github.com/jakevis/rinexprep/internal/ubx"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "convert":
		runConvert(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	case "version":
		fmt.Println("rinexprep v0.1.0-dev")
	default:
		printUsage()
		os.Exit(1)
	}
}

func runConvert(args []string) {
	fs := flag.NewFlagSet("convert", flag.ExitOnError)
	input := fs.String("input", "", "input UBX file (required)")
	output := fs.String("output", "", "output RINEX file (required)")
	format := fs.String("format", "rinex2", "output format: rinex2, rinex3, both")
	interval := fs.Int("interval", 30, "observation interval in seconds")
	fs.Parse(args)

	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "error: --input and --output are required")
		fs.Usage()
		os.Exit(1)
	}

	switch *format {
	case "rinex2", "rinex3", "both":
	default:
		fmt.Fprintf(os.Stderr, "error: --format must be rinex2, rinex3, or both (got %q)\n", *format)
		os.Exit(1)
	}

	// 1. Open and parse the UBX file.
	f, err := os.Open(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	epochPtrs, parseStats, err := ubx.Parse(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing UBX: %v\n", err)
		os.Exit(1)
	}

	epochs := make([]gnss.Epoch, len(epochPtrs))
	for i, p := range epochPtrs {
		epochs[i] = *p
	}

	fmt.Fprintf(os.Stderr, "Parsed %d UBX messages (%d RAWX → %d epochs)\n",
		parseStats.TotalMessages, parseStats.RawxMessages, len(epochs))

	if len(epochs) == 0 {
		fmt.Fprintln(os.Stderr, "warning: no RAWX epochs found in input file")
	}

	// 2. Auto-trim startup/teardown instability.
	autoTrimmed, trimResult := pipeline.AutoTrim(epochs, pipeline.DefaultAutoTrimConfig())
	if len(autoTrimmed) > 0 {
		fmt.Fprintf(os.Stderr, "Auto-trim: %s\n", trimResult.Reason)
		epochs = autoTrimmed
	}

	// 3. Run the normalization pipeline.
	cfg := pipeline.DefaultConfig()
	cfg.Normalize.IntervalSec = *interval
	cfg.Trim = pipeline.TrimConfig{} // trimming already applied
	processed, pipeStats := pipeline.Process(epochs, cfg)

	// 4. Build output metadata.
	meta := gnss.Metadata{
		MarkerName:   "UNKNOWN",
		MarkerNumber: "UNKNOWN",
		ReceiverType: "UNKNOWN",
		AntennaType:  "UNKNOWN NONE",
		Observer:     "UNKNOWN",
		Agency:       "UNKNOWN",
		Interval:     float64(*interval),
	}
	if len(processed) > 0 {
		meta.FirstEpoch = processed[0].Time
		meta.LastEpoch = processed[len(processed)-1].Time
	}

	warnings := meta.Validate()
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "WARNING: missing %s\n", w)
	}

	// 5. Write output file(s).
	switch *format {
	case "rinex2":
		if err := writeFile(*output, func(w *os.File) error {
			return rinex.WriteRinex2(w, meta, processed)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "error writing RINEX 2: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Output: %s\n", *output)

	case "rinex3":
		if err := writeFile(*output, func(w *os.File) error {
			return rinex.WriteRinex3(w, meta, processed)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "error writing RINEX 3: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Output: %s\n", *output)

	case "both":
		base := strings.TrimSuffix(*output, filepath.Ext(*output))
		obsPath := base + ".obs"
		rnxPath := base + ".rnx"

		if err := writeFile(obsPath, func(w *os.File) error {
			return rinex.WriteRinex2(w, meta, processed)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "error writing RINEX 2: %v\n", err)
			os.Exit(1)
		}
		if err := writeFile(rnxPath, func(w *os.File) error {
			return rinex.WriteRinex3(w, meta, processed)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "error writing RINEX 3: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Output: %s, %s\n", obsPath, rnxPath)
	}

	// 6. Print stats summary.
	fmt.Fprintf(os.Stderr, "\n--- Processing Summary ---\n")
	fmt.Fprintf(os.Stderr, "  Input epochs:    %d\n", pipeStats.InputEpochs)
	fmt.Fprintf(os.Stderr, "  After trim:      %d\n", pipeStats.AfterTrim)
	fmt.Fprintf(os.Stderr, "  After filter:    %d\n", pipeStats.AfterFilter)
	fmt.Fprintf(os.Stderr, "  After normalize: %d\n", pipeStats.AfterNormalize)
	if pipeStats.DroppedOffGrid > 0 {
		fmt.Fprintf(os.Stderr, "  Dropped (off-grid): %d\n", pipeStats.DroppedOffGrid)
	}
	if pipeStats.DroppedDuplicate > 0 {
		fmt.Fprintf(os.Stderr, "  Dropped (duplicate): %d\n", pipeStats.DroppedDuplicate)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 8080, "HTTP server port")
	dataDir := fs.String("data-dir", "./data", "directory for job data storage")
	fs.Parse(args)

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating data directory: %v\n", err)
		os.Exit(1)
	}

	srv := api.NewServer(*port, *dataDir)

	// Embed the React frontend if available.
	if distFS, err := frontend.DistFS(); err == nil {
		srv.SetFrontendFS(distFS)
	}

	fmt.Fprintf(os.Stderr, "Listening on :%d\n", *port)
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func writeFile(path string, fn func(*os.File) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return fn(f)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `RinexPrep - GNSS data processor for OPUS-compatible RINEX output

Usage:
  rinexprep <command> [options]

Commands:
  convert    Convert UBX to RINEX
  serve      Start the HTTP API server
  version    Print version

Use "rinexprep <command> --help" for more information.
`)
}
