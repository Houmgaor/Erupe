// Command patchtree generates the manifest for a game-file tree served to
// clients (the original launcher and mhf-outpost), for operators who host the
// tree on a separate web server rather than from Erupe itself. When
// API.PatchTree.Enabled is set, Erupe regenerates the manifest on startup and
// this tool is not needed.
//
// Usage:
//
//	go build -o patchtree ./cmd/patchtree/
//	./patchtree gen  --root /var/www/patch     # writes <root>/key.txt
//	./patchtree list --root /var/www/patch     # prints the manifest to stdout
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"erupe-ce/server/patchtree"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	root := fs.String("root", "patch", "directory holding mhfdat/{exe,dat}")
	_ = fs.Parse(os.Args[2:])

	switch os.Args[1] {
	case "gen":
		start := time.Now()
		n, err := patchtree.Generate(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s: %d files in %s\n", patchtree.ManifestFile, n, time.Since(start).Round(time.Millisecond))
	case "list":
		entries, err := patchtree.Scan(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := patchtree.WriteManifest(os.Stdout, entries); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: patchtree <gen|list> [--root DIR]")
}
