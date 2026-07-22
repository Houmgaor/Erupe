// Package binsync fetches quest, scenario, and Hunting Road (rengoku) JSON
// data from a remote HTTP host and installs it into BinPath, verifying each
// file's SHA-256 against a manifest and its content against Erupe's own
// compilers before it's ever placed where the server will read it.
package binsync

// ManifestVersion is the manifest schema version this build understands.
// Sync logs (but does not fail on) a Manifest whose Version differs, since
// the map-of-files shape itself is expected to stay stable across bumps.
const ManifestVersion = 1

// Manifest is the remote-hosted file describing every quest/scenario/road
// JSON file available for sync, keyed by path relative to BinPath.
type Manifest struct {
	Version int                  `json:"version"`
	Files   map[string]FileEntry `json:"files"`
}

// FileEntry describes one file listed in a Manifest.
type FileEntry struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
