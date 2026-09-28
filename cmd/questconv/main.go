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
	"encoding/binary"
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
		fmt.Printf(" verify_ok=%d verify_mismatch=%d verify_compile_error=%d", stats.verifyOK, stats.verifyMismatch, stats.verifyCompileErr)
	}
	fmt.Println()
	if stats.errors > 0 || stats.verifyMismatch > 0 || stats.verifyCompileErr > 0 {
		os.Exit(1)
	}
}

type exportStats struct {
	exported, errors, verifyOK, verifyMismatch, verifyCompileErr int
}

// verifyFunc recompiles jsonOut and compares the result with the original
// file. It returns "" when they match, else a description of the first
// difference; compileErr is set when the JSON can't be recompiled at all.
type verifyFunc func(jsonOut []byte) (mismatch string, compileErr error)

// convertFunc parses a .bin file's contents into JSON, with the check
// --verify runs on it.
type convertFunc func(raw []byte) (jsonOut []byte, verify verifyFunc, err error)

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

		jsonOut, check, err := convert(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR parsing %s: %v\n", srcPath, err)
			stats.errors++
			continue
		}

		if verify {
			switch mismatch, cerr := check(jsonOut); {
			case cerr != nil:
				fmt.Fprintf(os.Stderr, "VERIFY FAILED %s: recompile error: %v\n", srcPath, cerr)
				stats.verifyCompileErr++
			case mismatch != "":
				fmt.Fprintf(os.Stderr, "VERIFY MISMATCH %s: %s\n", srcPath, mismatch)
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

// exportQuest converts a quest. Quest .bin files are whole-file
// JKR-compressed on disk (see handlers_quest.go's loadQuestFile, which does
// the same unpack before parsing), while CompileQuestJSON produces the raw
// layout, so --verify compares against the decompressed original.
func exportQuest(raw []byte) ([]byte, verifyFunc, error) {
	data := decryption.UnpackSimple(raw) // no-op if the data isn't JKR
	q, err := channelserver.ParseQuestBinary(data)
	if err != nil {
		return nil, nil, err
	}
	jsonOut, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	verify := func(j []byte) (string, error) {
		recompiled, err := channelserver.CompileQuestJSON(j, "")
		if err != nil {
			return "", err
		}
		if !bytes.Equal(recompiled, data) {
			return fmt.Sprintf("recompiled output differs from parsed original (%d vs %d bytes)", len(recompiled), len(data)), nil
		}
		return "", nil
	}
	return jsonOut, verify, nil
}

// exportScenario converts a scenario. The container isn't compressed but
// some chunks are, and a recompressed chunk needn't match the retail
// encoder's bytes, so --verify compares the chunks after decompression.
func exportScenario(data []byte) ([]byte, verifyFunc, error) {
	s, err := channelserver.ParseScenarioBinary(data)
	if err != nil {
		return nil, nil, err
	}
	jsonOut, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	verify := func(j []byte) (string, error) {
		recompiled, err := channelserver.CompileScenarioJSON(j, "")
		if err != nil {
			return "", err
		}
		return diffScenarioChunks(data, recompiled), nil
	}
	return jsonOut, verify, nil
}

// diffScenarioChunks compares two scenario containers chunk by chunk, after
// JKR decompression, and describes the first difference ("" if none).
func diffScenarioChunks(orig, recompiled []byte) string {
	a, err := scenarioChunks(orig)
	if err != nil {
		return "original: " + err.Error()
	}
	b, err := scenarioChunks(recompiled)
	if err != nil {
		return "recompiled: " + err.Error()
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return fmt.Sprintf("chunk%d differs (%d vs %d bytes decompressed)", i, len(b[i]), len(a[i]))
		}
	}
	return ""
}

// scenarioChunks splits a scenario container (see docs/scenario-format.md)
// into its three chunks, decompressing JKR ones. Absent chunks are nil.
func scenarioChunks(data []byte) ([3][]byte, error) {
	var chunks [3][]byte
	if len(data) < 8 {
		return chunks, fmt.Errorf("container too short: %d bytes", len(data))
	}
	c0 := int(binary.BigEndian.Uint32(data[0:4]))
	c1 := int(binary.BigEndian.Uint32(data[4:8]))
	if c0 < 0 || c1 < 0 || 8+c0+c1 > len(data) {
		return chunks, fmt.Errorf("chunk sizes %d+%d overrun %d bytes", c0, c1, len(data))
	}
	chunks[0] = data[8 : 8+c0]
	chunks[1] = data[8+c0 : 8+c0+c1]
	if rest := data[8+c0+c1:]; len(rest) >= 4 {
		c2 := int(binary.BigEndian.Uint32(rest[0:4]))
		if c2 < 0 || 4+c2 > len(rest) {
			return chunks, fmt.Errorf("chunk2 size %d overruns %d bytes", c2, len(rest)-4)
		}
		chunks[2] = rest[4 : 4+c2]
	}
	for i := range chunks {
		if len(chunks[i]) > 0 {
			chunks[i] = decryption.UnpackSimple(chunks[i])
		} else {
			chunks[i] = nil
		}
	}
	return chunks, nil
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
