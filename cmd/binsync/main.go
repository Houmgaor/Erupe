// Command binsync downloads quest, scenario, and Hunting Road (rengoku)
// JSON data from a remote manifest into BinPath, verifying each file's
// SHA-256 hash and its content (via Erupe's own quest/scenario/road
// compilers) before installing it. See server/binsync for the sync
// implementation and docs/binsync-format.md for the manifest format.
//
// This is the headless counterpart to the setup wizard's "Sync Now" button
// — useful for scripted installs, Docker, and periodic re-syncs as quest
// data gets corrections over time.
//
// Usage:
//
//	go build -o binsync ./cmd/binsync/
//	./binsync --config config.json
//	./binsync --manifest-url https://data.mogapedia.fr/erupe/manifest.json --bin-path bin
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"erupe-ce/config"
	"erupe-ce/server/binsync"
)

// fileConfig is the minimal config.json subset binsync needs, read directly
// rather than through Viper (mirroring cmd/saveutil's dbConfig approach) so
// this tool doesn't depend on the server's full config-loading machinery.
type fileConfig struct {
	BinPath string `json:"BinPath"`
	BinSync struct {
		ManifestURL string `json:"ManifestURL"`
	} `json:"BinSync"`
}

func main() {
	configPath := flag.String("config", "config.json", "path to config.json (optional; flags below override its values)")
	manifestURL := flag.String("manifest-url", "", "manifest.json URL (overrides config.json's BinSync.ManifestURL)")
	binPath := flag.String("bin-path", "", "local directory to install into (overrides config.json's BinPath; auto-detected otherwise, see config.ResolveBinPath)")
	flag.Parse()

	var fc fileConfig
	if data, err := os.ReadFile(*configPath); err == nil {
		if err := json.Unmarshal(data, &fc); err != nil {
			fmt.Fprintf(os.Stderr, "binsync: parse %s: %v\n", *configPath, err)
			os.Exit(1)
		}
	}

	opts := binsync.Options{
		ManifestURL: fc.BinSync.ManifestURL,
		BinPath:     fc.BinPath,
	}
	if *manifestURL != "" {
		opts.ManifestURL = *manifestURL
	}
	if *binPath != "" {
		opts.BinPath = *binPath
	}
	if opts.BinPath == "" {
		opts.BinPath = "bin" // sentinel: let config.ResolveBinPath decide below
	}
	opts.BinPath = config.ResolveBinPath(opts.BinPath)
	if opts.ManifestURL == "" {
		fmt.Fprintln(os.Stderr, "binsync: no manifest URL — set BinSync.ManifestURL in config.json or pass --manifest-url")
		os.Exit(1)
	}

	report, err := binsync.Sync(context.Background(), opts, func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "binsync: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nfetched=%d skipped=%d failed=%d orphans=%d\n",
		len(report.Fetched), len(report.Skipped), len(report.Failed), len(report.Orphans))
	if len(report.Fetched) > 0 {
		fmt.Println("Note: Hunting Road (rengoku) changes only take effect after a server restart.")
	}
	if len(report.Failed) > 0 {
		os.Exit(1)
	}
}
