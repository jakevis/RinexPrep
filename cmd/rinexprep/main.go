package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "convert":
		fmt.Println("convert: not yet implemented")
	case "analyze":
		fmt.Println("analyze: not yet implemented")
	case "serve":
		fmt.Println("serve: not yet implemented")
	case "version":
		fmt.Println("rinexprep v0.1.0-dev")
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `RinexPrep - GNSS data processor for OPUS-compatible RINEX output

Usage:
  rinexprep <command> [options]

Commands:
  convert    Convert UBX to RINEX
  analyze    Run QC analysis on a file
  serve      Start the HTTP API server
  version    Print version

Use "rinexprep <command> --help" for more information.
`)
}
