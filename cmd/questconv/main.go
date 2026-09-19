// Command questconv is the producer-side counterpart to cmd/binsync: it
// bulk-converts existing retail .bin quest/scenario files to human-readable
// .json (via the same ParseQuestBinary/ParseScenarioBinary Erupe already
// uses to load them), and builds the manifest.json a binsync-compatible
// host serves. See docs/binsync-format.md for the manifest format.
//
// This is for whoever curates the remote data set — not something typical
// server operators need to run.
//
// Usage:
//
//	go build -o questconv ./cmd/questconv/
//	./questconv export   --bin-path game-data --out export/ --verify
//	./questconv manifest --dir export/ --out export/manifest.json
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"erupe-ce/common/decryption"
	"erupe-ce/config"
	"erupe-ce/server/binsync"
	"erupe-ce/server/channelserver"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "export":
		runExport(os.Args[2:])
	case "manifest":
		runManifest(os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  questconv export   --bin-path game-data --out export/ [--verify]
  questconv manifest --dir export/ --out export/manifest.json`)
}

// ── export ───────────────────────────────────────────────────────────────

func runExport(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	binPath := fs.String("bin-path", "", "source directory containing quests/ and scenarios/ .bin files (auto-detected if omitted, see config.ResolveBinPath)")
	out := fs.String("out", "export", "output directory for converted .json files")
	verify := fs.Bool("verify", false, "recompile each exported file and diff against the original .bin, warning on mismatch")
	_ = fs.Parse(args)

	src := *binPath
	if src == "" {
		src = "bin" // sentinel: let config.ResolveBinPath decide
	}
	src = config.ResolveBinPath(src)

	stats := exportStats{}
	exportDir(src, "quests", *out, *verify, exportQuest, &stats)
	exportDir(src, "scenarios", *out, *verify, exportScenario, &stats)

	fmt.Printf("\nexported=%d errors=%d", stats.exported, stats.errors)
	if *verify {
		fmt.Printf(" verify_ok=%d verify_mismatch=%d", stats.verifyOK, stats.verifyMismatch)
	}
	fmt.Println()
	if stats.errors > 0 || stats.verifyMismatch > 0 {
		os.Exit(1)
	}
}

type exportStats struct {
	exported, errors, verifyOK, verifyMismatch int
}

// convertFunc parses a .bin file's contents into JSON. canonical is what
// recompile(jsonOut) should be diffed against for --verify: for quests
// that's the decompressed bytes ParseQuestBinary actually parsed (quest
// .bin files are whole-file JKR-compressed, but CompileQuestJSON's output
// is the raw uncompressed layout), while for scenarios it's just the
// original file (the container itself isn't compressed — only specific
// sub-chunks are, which ParseScenarioBinary/CompileScenarioJSON already
// handle internally per docs/scenario-format.md).
type convertFunc func(raw []byte) (jsonOut, canonical []byte, recompile func([]byte) ([]byte, error), err error)

func exportDir(binPath, subdir, outDir string, verify bool, convert convertFunc, stats *exportStats) {
	srcDir := filepath.Join(binPath, subdir)
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return // absent directory is fine — not every install has, e.g., scenarios
	}

	dstDir := filepath.Join(outDir, subdir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bin") {
			continue
		}
		srcPath := filepath.Join(srcDir, e.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR reading %s: %v\n", srcPath, err)
			stats.errors++
			continue
		}

		jsonOut, canonical, recompile, err := convert(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR parsing %s: %v\n", srcPath, err)
			stats.errors++
			continue
		}

		if verify {
			switch recompiled, rerr := recompile(jsonOut); {
			case rerr != nil:
				fmt.Fprintf(os.Stderr, "VERIFY FAILED %s: recompile error: %v\n", srcPath, rerr)
				stats.verifyMismatch++
			case !bytes.Equal(recompiled, canonical):
				fmt.Fprintf(os.Stderr, "VERIFY MISMATCH %s: recompiled output differs from parsed original (%d vs %d bytes)\n", srcPath, len(recompiled), len(canonical))
				stats.verifyMismatch++
			default:
				stats.verifyOK++
			}
		}

		name := strings.TrimSuffix(e.Name(), ".bin") + ".json"
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR creating %s: %v\n", dstDir, err)
			stats.errors++
			continue
		}
		if err := os.WriteFile(filepath.Join(dstDir, name), jsonOut, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR writing %s: %v\n", name, err)
			stats.errors++
			continue
		}
		stats.exported++
	}
}

func exportQuest(raw []byte) ([]byte, []byte, func([]byte) ([]byte, error), error) {
	// Quest .bin files are whole-file JKR-compressed on disk (see
	// handlers_quest.go's loadQuestFile, which does the same unpack before
	// parsing); UnpackSimple is a no-op if the data isn't actually JKR data.
	data := decryption.UnpackSimple(raw)
	q, err := channelserver.ParseQuestBinary(data)
	if err != nil {
		return nil, nil, nil, err
	}
	jsonOut, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return nil, nil, nil, err
	}
	recompile := func(j []byte) ([]byte, error) { return channelserver.CompileQuestJSON(j, "") }
	return jsonOut, data, recompile, nil
}

func exportScenario(data []byte) ([]byte, []byte, func([]byte) ([]byte, error), error) {
	s, err := channelserver.ParseScenarioBinary(data)
	if err != nil {
		return nil, nil, nil, err
	}
	jsonOut, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, nil, nil, err
	}
	recompile := func(j []byte) ([]byte, error) { return channelserver.CompileScenarioJSON(j, "") }
	return jsonOut, data, recompile, nil
}

// ── manifest ─────────────────────────────────────────────────────────────

func runManifest(args []string) {
	fs := flag.NewFlagSet("manifest", flag.ExitOnError)
	dir := fs.String("dir", "export", "directory of converted .json files to hash")
	out := fs.String("out", "manifest.json", "output path for manifest.json")
	_ = fs.Parse(args)

	files := make(map[string]binsync.FileEntry)
	for _, subdir := range []string{"quests", "scenarios"} {
		entries, err := os.ReadDir(filepath.Join(*dir, subdir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			rel := subdir + "/" + e.Name()
			entry, err := hashFile(filepath.Join(*dir, subdir, e.Name()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR hashing %s: %v\n", rel, err)
				os.Exit(1)
			}
			files[rel] = entry
		}
	}
	rengokuPath := filepath.Join(*dir, "rengoku_data.json")
	if _, err := os.Stat(rengokuPath); err == nil {
		entry, err := hashFile(rengokuPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR hashing rengoku_data.json: %v\n", err)
			os.Exit(1)
		}
		files["rengoku_data.json"] = entry
	}

	manifest := binsync.Manifest{Version: binsync.ManifestVersion, Files: files}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR encoding manifest: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR writing %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s with %d file(s)\n", *out, len(files))
}

func hashFile(path string) (binsync.FileEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return binsync.FileEntry{}, err
	}
	sum := sha256.Sum256(data)
	return binsync.FileEntry{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}, nil
}
