package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

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
	case "batch":
		runBatch(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	case "version":
		fmt.Println("rinexprep v0.1.0-dev")
	default:
		printUsage()
		os.Exit(1)
	}
}

// convertResult holds the outcome of processing a single UBX file.
type convertResult struct {
	InputFile  string
	OutputFile string
	Epochs     int
	Duration   time.Duration
	ObsCount   int
	Err        error
}

// processUBXFile runs the full pipeline on a single UBX file and writes RINEX output.
func processUBXFile(inputPath, outputPath, format string, interval int) convertResult {
	result := convertResult{InputFile: inputPath, OutputFile: outputPath}

	// 1. Open and parse the UBX file.
	f, err := os.Open(inputPath)
	if err != nil {
		result.Err = fmt.Errorf("opening file: %w", err)
		return result
	}
	defer f.Close()

	epochPtrs, parseStats, err := ubx.Parse(f)
	if err != nil {
		result.Err = fmt.Errorf("parsing UBX: %w", err)
		return result
	}

	epochs := make([]gnss.Epoch, len(epochPtrs))
	for i, p := range epochPtrs {
		epochs[i] = *p
	}

	if len(epochs) == 0 {
		result.Err = fmt.Errorf("no RAWX epochs found in input file")
		return result
	}

	// 2. Apply receiver clock correction (RTKLIB -TADJ=0.1 equivalent).
	epochs = pipeline.CorrectClockBias(epochs, pipeline.ClockCorrConfig{TADJ: 0.1})

	// 3. Auto-trim startup/teardown instability.
	autoTrimmed, _ := pipeline.AutoTrim(epochs, pipeline.DefaultAutoTrimConfig())
	if len(autoTrimmed) > 0 {
		epochs = autoTrimmed
	}

	// 4. Run the normalization pipeline.
	cfg := pipeline.DefaultConfig()
	cfg.Normalize.IntervalSec = interval
	cfg.Trim = pipeline.TrimConfig{}
	processed, _ := pipeline.Process(epochs, cfg)

	// 5. Build output metadata.
	meta := gnss.Metadata{
		MarkerName:   "UNKNOWN",
		MarkerNumber: "UNKNOWN",
		ReceiverType: "UNKNOWN",
		AntennaType:  "UNKNOWN NONE",
		Observer:     "UNKNOWN",
		Agency:       "UNKNOWN",
		Interval:     float64(interval),
	}
	if len(processed) > 0 {
		meta.FirstEpoch = processed[0].Time
		meta.LastEpoch = processed[len(processed)-1].Time
	}
	if parseStats.BestPosition != nil {
		meta.ApproxX, meta.ApproxY, meta.ApproxZ = parseStats.BestPosition.ECEF()
	}
	meta.Validate()

	// 6. Write output file(s).
	switch format {
	case "rinex2":
		if err := writeFile(outputPath, func(w *os.File) error {
			return rinex.WriteRinex2(w, meta, processed)
		}); err != nil {
			result.Err = fmt.Errorf("writing RINEX 2: %w", err)
			return result
		}
	case "rinex3":
		if err := writeFile(outputPath, func(w *os.File) error {
			return rinex.WriteRinex3(w, meta, processed)
		}); err != nil {
			result.Err = fmt.Errorf("writing RINEX 3: %w", err)
			return result
		}
	case "both":
		base := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
		obsPath := base + ".obs"
		rnxPath := base + ".rnx"
		if err := writeFile(obsPath, func(w *os.File) error {
			return rinex.WriteRinex2(w, meta, processed)
		}); err != nil {
			result.Err = fmt.Errorf("writing RINEX 2: %w", err)
			return result
		}
		if err := writeFile(rnxPath, func(w *os.File) error {
			return rinex.WriteRinex3(w, meta, processed)
		}); err != nil {
			result.Err = fmt.Errorf("writing RINEX 3: %w", err)
			return result
		}
	}

	// Populate result stats.
	result.Epochs = len(processed)
	if len(processed) >= 2 {
		first := processed[0].Time.UnixNanos()
		last := processed[len(processed)-1].Time.UnixNanos()
		result.Duration = time.Duration(last - first)
	}
	for _, ep := range processed {
		for _, sat := range ep.Satellites {
			result.ObsCount += len(sat.Signals)
		}
	}

	return result
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

	result := processUBXFile(*input, *output, *format, *interval)
	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", result.Err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Output: %s\n", *output)
	fmt.Fprintf(os.Stderr, "\n--- Processing Summary ---\n")
	fmt.Fprintf(os.Stderr, "  Epochs:   %d\n", result.Epochs)
	fmt.Fprintf(os.Stderr, "  Duration: %s\n", result.Duration.Truncate(time.Second))
	fmt.Fprintf(os.Stderr, "  Obs:      %d\n", result.ObsCount)
}

func runBatch(args []string) {
	fs := flag.NewFlagSet("batch", flag.ExitOnError)
	inputDir := fs.String("input-dir", "", "directory containing .ubx files (required)")
	outputDir := fs.String("output-dir", "", "directory for RINEX output (required)")
	format := fs.String("format", "rinex3", "output format: rinex2, rinex3, both")
	interval := fs.Int("interval", 30, "observation interval in seconds")
	fs.Parse(args)

	if *inputDir == "" || *outputDir == "" {
		fmt.Fprintln(os.Stderr, "error: --input-dir and --output-dir are required")
		fs.Usage()
		os.Exit(1)
	}

	switch *format {
	case "rinex2", "rinex3", "both":
	default:
		fmt.Fprintf(os.Stderr, "error: --format must be rinex2, rinex3, or both (got %q)\n", *format)
		os.Exit(1)
	}

	// Scan for .ubx files.
	matches, err := filepath.Glob(filepath.Join(*inputDir, "*.ubx"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning input directory: %v\n", err)
		os.Exit(1)
	}
	if len(matches) == 0 {
		fmt.Fprintln(os.Stderr, "error: no .ubx files found in input directory")
		os.Exit(1)
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Derive output extension from format.
	ext := ".rnx"
	switch *format {
	case "rinex2":
		ext = ".obs"
	case "both":
		ext = ".rnx" // primary extension; both writes .obs and .rnx
	}

	// Process each file sequentially.
	results := make([]convertResult, 0, len(matches))
	for i, inputPath := range matches {
		base := strings.TrimSuffix(filepath.Base(inputPath), ".ubx")
		outputPath := filepath.Join(*outputDir, base+ext)

		fmt.Fprintf(os.Stderr, "[%d/%d] Processing %s ...\n", i+1, len(matches), filepath.Base(inputPath))

		result := processUBXFile(inputPath, outputPath, *format, *interval)
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR: %v\n", result.Err)
		} else {
			fmt.Fprintf(os.Stderr, "  OK: %d epochs, %s\n", result.Epochs, result.Duration.Truncate(time.Second))
		}
		results = append(results, result)
	}

	// Print summary table.
	fmt.Fprintf(os.Stderr, "\n--- Batch Summary ---\n")
	tw := tabwriter.NewWriter(os.Stderr, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "FILE\tSTATUS\tEPOCHS\tDURATION\tOBS")
	failed := 0
	for _, r := range results {
		name := filepath.Base(r.InputFile)
		if r.Err != nil {
			fmt.Fprintf(tw, "%s\tFAILED\t-\t-\t-\n", name)
			failed++
		} else {
			fmt.Fprintf(tw, "%s\tOK\t%d\t%s\t%d\n", name, r.Epochs, r.Duration.Truncate(time.Second), r.ObsCount)
		}
	}
	tw.Flush()

	fmt.Fprintf(os.Stderr, "\nProcessed %d files: %d succeeded, %d failed\n",
		len(results), len(results)-failed, failed)

	if failed > 0 {
		os.Exit(1)
	}
}

func runServe(args []string) {
	serveFlags := flag.NewFlagSet("serve", flag.ExitOnError)
	port := serveFlags.Int("port", 8080, "HTTP server port")
	dataDir := serveFlags.String("data-dir", "./data", "directory for job data storage")
	jsonLogs := serveFlags.Bool("json-logs", false, "Output structured JSON logs")
	serveFlags.Parse(args)

	api.SetupLogger(*jsonLogs)

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating data directory: %v\n", err)
		os.Exit(1)
	}

	slog.Info("RinexPrep starting",
		"version", "0.1.0",
		"port", *port,
		"data_dir", *dataDir,
		"cleanup_interval", "30m",
	)

	srv := api.NewServer(*port, *dataDir)

	// Embed the React frontend if available.
	if distFS, err := frontend.DistFS(); err == nil {
		srv.SetFrontendFS(distFS)
	}

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
  convert    Convert a single UBX file to RINEX
  batch      Convert multiple UBX files in a directory
  serve      Start the HTTP API server
  version    Print version

Use "rinexprep <command> --help" for more information.
`)
}
