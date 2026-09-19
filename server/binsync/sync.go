package binsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"erupe-ce/server/channelserver"
)

const defaultTimeout = 30 * time.Second

// Options configures a Sync run.
type Options struct {
	// ManifestURL is the HTTP(S) URL of the remote manifest.json. Individual
	// file URLs are resolved relative to it (i.e. relative to the directory
	// containing manifest.json).
	ManifestURL string
	// BinPath is the local directory files are installed into, mirroring
	// config.BinPath (e.g. "bin").
	BinPath string
	// HTTPClient overrides the client used for all requests. Defaults to a
	// client with a defaultTimeout when nil.
	HTTPClient *http.Client
}

// FailedFile records one manifest entry that could not be installed.
type FailedFile struct {
	Path string
	Err  string
}

// Report summarizes the outcome of a Sync run.
type Report struct {
	Fetched []string
	Skipped []string
	Failed  []FailedFile
	Orphans []string
}

// Sync downloads Manifest.Files that are missing or changed, verifies each
// one's SHA-256 against the manifest, validates its content against Erupe's
// own quest/scenario/road compilers, and installs valid files atomically
// into opts.BinPath. A file that fails hash verification or validation is
// discarded without ever touching an existing file at that path. Files
// present locally but absent from the manifest are reported as orphans and
// left untouched — sync never deletes.
func Sync(ctx context.Context, opts Options, logf func(string, ...any)) (Report, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}

	manifest, err := fetchManifest(ctx, client, opts.ManifestURL)
	if err != nil {
		return Report{}, fmt.Errorf("fetch manifest: %w", err)
	}
	if manifest.Version != ManifestVersion {
		logf("manifest version %d differs from supported version %d; continuing", manifest.Version, ManifestVersion)
	}

	var report Report
	for relPath, entry := range manifest.Files {
		localPath := filepath.Join(opts.BinPath, filepath.FromSlash(relPath))

		if matches, err := localFileMatches(localPath, entry.SHA256); err == nil && matches {
			report.Skipped = append(report.Skipped, relPath)
			logf("skip %s (up to date)", relPath)
			continue
		}

		if err := fetchAndInstall(ctx, client, opts.ManifestURL, relPath, localPath, entry); err != nil {
			logf("failed %s: %v", relPath, err)
			report.Failed = append(report.Failed, FailedFile{Path: relPath, Err: err.Error()})
			continue
		}

		logf("fetched %s", relPath)
		report.Fetched = append(report.Fetched, relPath)
	}

	report.Orphans = findOrphans(opts.BinPath, manifest)
	for _, o := range report.Orphans {
		logf("orphan (not in manifest, left untouched): %s", o)
	}

	sort.Strings(report.Fetched)
	sort.Strings(report.Skipped)
	sort.Strings(report.Orphans)

	return report, nil
}

func fetchManifest(ctx context.Context, client *http.Client, manifestURL string) (Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return Manifest{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Manifest{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("unexpected status %s", resp.Status)
	}
	var m Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return m, nil
}

func localFileMatches(localPath, wantSHA256 string) (bool, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), wantSHA256), nil
}

func resolveFileURL(manifestURL, relPath string) (string, error) {
	base, err := url.Parse(manifestURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(relPath)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

// fetchAndInstall downloads relPath, verifies its hash, validates its
// content, and only then writes it to localPath via a temp-file-plus-rename
// so a crash mid-write can never leave a truncated file in place.
func fetchAndInstall(ctx context.Context, client *http.Client, manifestURL, relPath, localPath string, entry FileEntry) error {
	fileURL, err := resolveFileURL(manifestURL, relPath)
	if err != nil {
		return fmt.Errorf("resolve URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	h := sha256.New()
	data, err := io.ReadAll(io.TeeReader(resp.Body, h))
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, entry.SHA256) {
		return fmt.Errorf("sha256 mismatch: manifest says %s, downloaded %s", entry.SHA256, got)
	}

	if err := validate(relPath, data); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(localPath), filepath.Base(localPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, localPath); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	return nil
}

// validate runs data through the same compiler Erupe uses at load time,
// so a file that's byte-identical to the manifest's hash but semantically
// malformed (truncated, wrong shape, corrupted mid-authoring) is still
// rejected before it ever reaches BinPath. Only the three known data kinds
// are accepted; anything else is refused rather than installed blind.
func validate(relPath string, data []byte) error {
	clean := filepath.ToSlash(relPath)
	switch {
	case strings.HasPrefix(clean, "quests/") && strings.HasSuffix(clean, ".json"):
		_, err := channelserver.CompileQuestJSON(data, "en")
		return err
	case strings.HasPrefix(clean, "scenarios/") && strings.HasSuffix(clean, ".json"):
		_, err := channelserver.CompileScenarioJSON(data, "en")
		return err
	case clean == "rengoku_data.json":
		var cfg channelserver.RengokuConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode rengoku config: %w", err)
		}
		_, err := channelserver.BuildRengokuBinary(cfg)
		return err
	default:
		return fmt.Errorf("no validator for %q; refusing to install unrecognized file type", relPath)
	}
}

// findOrphans reports local .json files under quests/, scenarios/, and
// rengoku_data.json that aren't referenced by the manifest. It never deletes
// — an operator may have hand-authored custom content.
func findOrphans(binPath string, manifest Manifest) []string {
	known := make(map[string]bool, len(manifest.Files))
	for p := range manifest.Files {
		known[filepath.ToSlash(p)] = true
	}

	var orphans []string
	for _, dir := range []string{"quests", "scenarios"} {
		entries, err := os.ReadDir(filepath.Join(binPath, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			rel := path.Join(dir, e.Name())
			if !known[rel] {
				orphans = append(orphans, rel)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(binPath, "rengoku_data.json")); err == nil && !known["rengoku_data.json"] {
		orphans = append(orphans, "rengoku_data.json")
	}
	return orphans
}
