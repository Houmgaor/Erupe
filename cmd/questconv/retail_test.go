package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// retailBaseline is how many files survived the .bin -> JSON -> .bin round
// trip on the full retail set when #40 was opened. Raise it as the JSON
// formats become lossless; the goal is ok == files.
var retailBaseline = map[string]struct{ files, ok int }{
	"quests":    {54977, 0},
	"scenarios": {145376, 9166},
}

// TestRetailRoundTrip runs the --verify round trip over real game data and
// fails if fewer files survive than retailBaseline (#40). Retail data isn't
// in the repository, so it is opt-in:
//
//	ERUPE_RETAIL_BIN=bin go test ./cmd/questconv -run TestRetailRoundTrip -v
func TestRetailRoundTrip(t *testing.T) {
	binPath := os.Getenv("ERUPE_RETAIL_BIN")
	if binPath == "" {
		t.Skip("set ERUPE_RETAIL_BIN to a directory with quests/ and scenarios/ .bin files")
	}
	for _, kind := range []struct {
		subdir  string
		convert convertFunc
	}{
		{"quests", exportQuest},
		{"scenarios", exportScenario},
	} {
		t.Run(kind.subdir, func(t *testing.T) {
			files, err := filepath.Glob(filepath.Join(binPath, kind.subdir, "*.bin"))
			if err != nil {
				t.Fatal(err)
			}
			if len(files) == 0 {
				t.Skipf("no .bin files in %s", filepath.Join(binPath, kind.subdir))
			}
			var ok, mismatches, compileErrs, parseErrs int
			reasons := map[string]int{}
			for _, f := range files {
				data, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				jsonOut, verify, err := kind.convert(data)
				if err != nil {
					parseErrs++
					continue
				}
				switch mismatch, cerr := verify(jsonOut); {
				case cerr != nil:
					compileErrs++
				case mismatch != "":
					mismatches++
					reasons[strings.SplitN(mismatch, " (", 2)[0]]++
				default:
					ok++
				}
			}
			t.Logf("%d files: %d ok, %d mismatch, %d compile error, %d parse error",
				len(files), ok, mismatches, compileErrs, parseErrs)
			for reason, n := range reasons {
				t.Logf("  %6d  %s", n, reason)
			}

			base := retailBaseline[kind.subdir]
			switch {
			case len(files) != base.files:
				t.Logf("not the reference set (%d files), baseline not checked", base.files)
			case ok < base.ok:
				t.Errorf("%d files round-trip, baseline is %d: fidelity regressed", ok, base.ok)
			case ok > base.ok:
				t.Logf("%d files round-trip, above the baseline of %d: raise retailBaseline", ok, base.ok)
			}
		})
	}
}
