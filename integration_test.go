package main

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jakevis/rinexprep/internal/gnss"
	"github.com/jakevis/rinexprep/internal/pipeline"
	"github.com/jakevis/rinexprep/internal/rinex"
	"github.com/jakevis/rinexprep/internal/ubx"
)

// TestIntegrationSample30s is the primary regression test for the full
// UBX→RINEX conversion pipeline. It processes testdata/fixtures/sample_30s.ubx
// (a real ~2.5hr F9P capture) and compares both RINEX 2 and RINEX 3 output
// against OPUS-validated golden reference files.
//
// The golden references were generated from the same pipeline and the RINEX 3
// file was successfully submitted to NGS OPUS with these results:
//
//	OBS USED: 4708/4981 (95%), FIXED AMB: 28/28 (100%), RMS: 0.012m
//	START: 2026/04/14 00:52:00, STOP: 2026/04/14 03:13:00
func TestIntegrationSample30s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	const (
		ubxPath    = "testdata/fixtures/sample_30s.ubx"
		refV2Path  = "testdata/fixtures/sample_30s_v2.ref.obs"
		refV3Path  = "testdata/fixtures/sample_30s_v3.ref.rnx"
		interval   = 30
		wantEpochs = 283
		wantObs    = 5009
	)

	// 1. Parse UBX file.
	f, err := os.Open(ubxPath)
	if err != nil {
		t.Fatalf("open UBX: %v", err)
	}
	defer f.Close()

	epochPtrs, _, err := ubx.Parse(f)
	if err != nil {
		t.Fatalf("parse UBX: %v", err)
	}
	epochs := make([]gnss.Epoch, len(epochPtrs))
	for i, p := range epochPtrs {
		epochs[i] = *p
	}
	if len(epochs) == 0 {
		t.Fatal("no RAWX epochs parsed")
	}

	// 2. Clock correction.
	epochs = pipeline.CorrectClockBias(epochs, pipeline.ClockCorrConfig{TADJ: 0.1})

	// 3. Auto-trim.
	autoTrimmed, _ := pipeline.AutoTrim(epochs, pipeline.DefaultAutoTrimConfig())
	if len(autoTrimmed) > 0 {
		epochs = autoTrimmed
	}

	// 4. Pipeline processing.
	cfg := pipeline.DefaultConfig()
	cfg.Normalize.IntervalSec = interval
	cfg.Trim = pipeline.TrimConfig{}
	processed, _ := pipeline.Process(epochs, cfg)

	// 5. Build metadata (same as CLI defaults).
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
	meta.Validate()

	// --- Semantic assertions ---

	t.Run("epoch_count", func(t *testing.T) {
		if len(processed) != wantEpochs {
			t.Errorf("epoch count = %d, want %d", len(processed), wantEpochs)
		}
	})

	t.Run("observation_count", func(t *testing.T) {
		obs := 0
		for _, ep := range processed {
			for _, sat := range ep.Satellites {
				obs += len(sat.Signals)
			}
		}
		if obs != wantObs {
			t.Errorf("observation count = %d, want %d", obs, wantObs)
		}
	})

	t.Run("duration", func(t *testing.T) {
		if len(processed) < 2 {
			t.Fatal("not enough epochs")
		}
		first := processed[0].Time.UnixNanos()
		last := processed[len(processed)-1].Time.UnixNanos()
		dur := time.Duration(last - first)
		wantDur := 2*time.Hour + 21*time.Minute
		if dur != wantDur {
			t.Errorf("duration = %v, want %v", dur, wantDur)
		}
	})

	t.Run("first_epoch_time", func(t *testing.T) {
		ft := processed[0].Time
		year, month, day, hour, min, sec := rinex.GNSSTimeToCalendar(ft)
		if year != 2026 || month != 4 || day != 14 || hour != 0 || min != 52 || sec != 30.0 {
			t.Errorf("first epoch = %d-%02d-%02d %02d:%02d:%06.3f, want 2026-04-14 00:52:30.000",
				year, month, day, hour, min, sec)
		}
	})

	t.Run("last_epoch_time", func(t *testing.T) {
		lt := processed[len(processed)-1].Time
		year, month, day, hour, min, sec := rinex.GNSSTimeToCalendar(lt)
		if year != 2026 || month != 4 || day != 14 || hour != 3 || min != 13 || sec != 30.0 {
			t.Errorf("last epoch = %d-%02d-%02d %02d:%02d:%06.3f, want 2026-04-14 03:13:30.000",
				year, month, day, hour, min, sec)
		}
	})

	// --- Full output comparison against golden references ---

	t.Run("rinex2_golden", func(t *testing.T) {
		var buf bytes.Buffer
		if err := rinex.WriteRinex2(&buf, meta, processed); err != nil {
			t.Fatalf("WriteRinex2: %v", err)
		}
		ref, err := os.ReadFile(refV2Path)
		if err != nil {
			t.Fatalf("read reference: %v", err)
		}
		compareRINEX(t, string(ref), buf.String())
	})

	t.Run("rinex3_golden", func(t *testing.T) {
		var buf bytes.Buffer
		if err := rinex.WriteRinex3(&buf, meta, processed); err != nil {
			t.Fatalf("WriteRinex3: %v", err)
		}
		ref, err := os.ReadFile(refV3Path)
		if err != nil {
			t.Fatalf("read reference: %v", err)
		}
		compareRINEX(t, string(ref), buf.String())
	})

	// --- RINEX 2 specific: must use C2, never P2 ---

	t.Run("rinex2_c2_not_p2", func(t *testing.T) {
		ref, _ := os.ReadFile(refV2Path)
		content := string(ref)
		if strings.Contains(content, "    P2") {
			t.Error("RINEX 2 must use C2, not P2 — OPUS will reject P2 for L2C data")
		}
		if !strings.Contains(content, "    C2") {
			t.Error("RINEX 2 must contain C2 observation type")
		}
	})
}

// compareRINEX compares two RINEX files line-by-line, skipping the
// PGM / RUN BY / DATE header line which contains a build timestamp.
func compareRINEX(t *testing.T, want, got string) {
	t.Helper()

	wantLines := splitLines(want)
	gotLines := splitLines(got)

	if len(wantLines) != len(gotLines) {
		t.Errorf("line count: got %d, want %d", len(gotLines), len(wantLines))
		// Continue to show first diff
	}

	n := len(wantLines)
	if len(gotLines) < n {
		n = len(gotLines)
	}

	diffs := 0
	const maxDiffs = 10
	for i := 0; i < n; i++ {
		// Skip the PGM / RUN BY / DATE line — it has a timestamp that changes.
		if strings.Contains(wantLines[i], "PGM / RUN BY / DATE") {
			continue
		}
		if wantLines[i] != gotLines[i] {
			diffs++
			if diffs <= maxDiffs {
				t.Errorf("line %d diff:\n  want: %q\n  got:  %q", i+1, wantLines[i], gotLines[i])
			}
		}
	}
	if diffs > maxDiffs {
		t.Errorf("... and %d more diffs (total: %d)", diffs-maxDiffs, diffs)
	}
}

func splitLines(s string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}
