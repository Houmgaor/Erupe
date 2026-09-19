package binsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// validQuestJSON is a minimal but complete quest, mirroring the fixture used
// in server/channelserver/quest_json_test.go, so CompileQuestJSON accepts it.
const validQuestJSON = `{
	"quest_id": 1,
	"title": "Test Quest",
	"description": "A test quest.",
	"text_main": "Hunt the Rathalos.",
	"text_sub_a": "",
	"text_sub_b": "",
	"success_cond": "Slay the Rathalos.",
	"fail_cond": "Time runs out or all hunters faint.",
	"contractor": "Guild Master",
	"monster_size_multi": 100,
	"stat_table_1": 0,
	"main_rank_points": 120,
	"sub_a_rank_points": 60,
	"sub_b_rank_points": 0,
	"fee": 500,
	"reward_main": 5000,
	"reward_sub_a": 1000,
	"reward_sub_b": 0,
	"time_limit_minutes": 50,
	"map": 2,
	"rank_band": 0,
	"objective_main": {"type": "hunt", "target": 11, "count": 1},
	"objective_sub_a": {"type": "deliver", "target": 149, "count": 3},
	"objective_sub_b": {"type": "none"},
	"large_monsters": [
		{"id": 11, "spawn_amount": 1, "spawn_stage": 5, "orientation": 180, "x": 1500.0, "y": 0.0, "z": -2000.0}
	],
	"rewards": [
		{
			"table_id": 1,
			"items": [
				{"rate": 50, "item": 149, "quantity": 1},
				{"rate": 30, "item": 153, "quantity": 1}
			]
		}
	],
	"supply_main": [
		{"item": 1, "quantity": 5}
	],
	"stages": [
		{"stage_id": 2}
	]
}`

// invalidQuestJSON parses as JSON but fails CompileQuestJSON's semantic
// checks (unknown objective type), mirroring
// TestCompileQuestJSON_BadObjectiveType in the channelserver package.
const invalidQuestJSON = `{
	"quest_id": 1,
	"objective_main": {"type": "not_a_real_objective_type"}
}`

const validRengokuJSON = `{}`

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// newTestServer serves manifest.json plus the given files at their manifest
// paths. If manifestFiles is nil, the manifest is derived from files with
// correct hashes; passing manifestFiles explicitly lets a test declare a
// hash that doesn't match what's actually served.
func newTestServer(t *testing.T, files map[string][]byte, manifestFiles map[string]FileEntry) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for relPath, content := range files {
		content := content
		mux.HandleFunc("/"+relPath, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(content)
		})
	}
	if manifestFiles == nil {
		manifestFiles = make(map[string]FileEntry, len(files))
		for relPath, content := range files {
			manifestFiles[relPath] = FileEntry{SHA256: hashOf(content), Size: int64(len(content))}
		}
	}
	manifest := Manifest{Version: ManifestVersion, Files: manifestFiles}
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(manifest)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSync_FetchesAndValidatesNewFiles(t *testing.T) {
	binPath := t.TempDir()
	files := map[string][]byte{
		"quests/00001d0.json": []byte(validQuestJSON),
		"rengoku_data.json":   []byte(validRengokuJSON),
	}
	srv := newTestServer(t, files, nil)

	report, err := Sync(context.Background(), Options{
		ManifestURL: srv.URL + "/manifest.json",
		BinPath:     binPath,
	}, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(report.Failed) != 0 {
		t.Fatalf("unexpected failures: %+v", report.Failed)
	}
	if len(report.Fetched) != 2 {
		t.Fatalf("Fetched = %v, want 2 entries", report.Fetched)
	}

	for relPath, want := range files {
		got, err := os.ReadFile(filepath.Join(binPath, relPath))
		if err != nil {
			t.Fatalf("reading installed %s: %v", relPath, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s content mismatch after sync", relPath)
		}
	}
}

func TestSync_SkipsFileWithMatchingHash(t *testing.T) {
	binPath := t.TempDir()
	content := []byte(validQuestJSON)
	relPath := "quests/00001d0.json"

	if err := os.MkdirAll(filepath.Join(binPath, "quests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binPath, relPath), content, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, map[string][]byte{relPath: content}, nil)

	report, err := Sync(context.Background(), Options{
		ManifestURL: srv.URL + "/manifest.json",
		BinPath:     binPath,
	}, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(report.Fetched) != 0 {
		t.Fatalf("Fetched = %v, want none (should have been skipped)", report.Fetched)
	}
	if len(report.Skipped) != 1 || report.Skipped[0] != relPath {
		t.Fatalf("Skipped = %v, want [%s]", report.Skipped, relPath)
	}
}

func TestSync_RejectsHashMismatchWithoutWriting(t *testing.T) {
	binPath := t.TempDir()
	relPath := "quests/00001d0.json"
	content := []byte(validQuestJSON)

	// Manifest claims a hash that doesn't match the actually-served content.
	badManifest := map[string]FileEntry{
		relPath: {SHA256: hashOf([]byte("not the real content")), Size: int64(len(content))},
	}
	srv := newTestServer(t, map[string][]byte{relPath: content}, badManifest)

	report, err := Sync(context.Background(), Options{
		ManifestURL: srv.URL + "/manifest.json",
		BinPath:     binPath,
	}, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(report.Failed) != 1 || report.Failed[0].Path != relPath {
		t.Fatalf("Failed = %+v, want one entry for %s", report.Failed, relPath)
	}
	if _, err := os.Stat(filepath.Join(binPath, relPath)); !os.IsNotExist(err) {
		t.Fatalf("expected %s not to be written after hash mismatch, stat err = %v", relPath, err)
	}
}

func TestSync_RejectsFailedValidationAndKeepsExistingFile(t *testing.T) {
	binPath := t.TempDir()
	relPath := "quests/00001d0.json"

	if err := os.MkdirAll(filepath.Join(binPath, "quests"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := []byte(validQuestJSON)
	if err := os.WriteFile(filepath.Join(binPath, relPath), existing, 0o644); err != nil {
		t.Fatal(err)
	}

	// The server now offers a different (hash-matching-to-itself, but
	// semantically invalid) version.
	bad := []byte(invalidQuestJSON)
	srv := newTestServer(t, map[string][]byte{relPath: bad}, nil)

	report, err := Sync(context.Background(), Options{
		ManifestURL: srv.URL + "/manifest.json",
		BinPath:     binPath,
	}, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(report.Failed) != 1 || report.Failed[0].Path != relPath {
		t.Fatalf("Failed = %+v, want one entry for %s", report.Failed, relPath)
	}

	got, err := os.ReadFile(filepath.Join(binPath, relPath))
	if err != nil {
		t.Fatalf("reading %s: %v", relPath, err)
	}
	if string(got) != string(existing) {
		t.Fatalf("existing good file was overwritten by a failed-validation download")
	}
}

func TestSync_ReportsOrphansWithoutDeleting(t *testing.T) {
	binPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(binPath, "quests"), 0o755); err != nil {
		t.Fatal(err)
	}
	orphanPath := filepath.Join(binPath, "quests", "99999_custom.json")
	if err := os.WriteFile(orphanPath, []byte(validQuestJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Manifest references a different file entirely.
	srv := newTestServer(t, map[string][]byte{"quests/00001d0.json": []byte(validQuestJSON)}, nil)

	report, err := Sync(context.Background(), Options{
		ManifestURL: srv.URL + "/manifest.json",
		BinPath:     binPath,
	}, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(report.Orphans) != 1 || report.Orphans[0] != "quests/99999_custom.json" {
		t.Fatalf("Orphans = %v, want [quests/99999_custom.json]", report.Orphans)
	}
	if _, err := os.Stat(orphanPath); err != nil {
		t.Fatalf("orphan file was removed: %v", err)
	}
}

func TestSync_RejectsUnrecognizedFileType(t *testing.T) {
	binPath := t.TempDir()
	relPath := "quests/00001d0.txt" // not .json under quests/, so validate() has no matching case
	srv := newTestServer(t, map[string][]byte{relPath: []byte("hello")}, nil)

	report, err := Sync(context.Background(), Options{
		ManifestURL: srv.URL + "/manifest.json",
		BinPath:     binPath,
	}, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(report.Failed) != 1 || report.Failed[0].Path != relPath {
		t.Fatalf("Failed = %+v, want one entry for %s", report.Failed, relPath)
	}
}
