package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"erupe-ce/server/patchtree"

	"github.com/gorilla/mux"
)

// TestMountPatchTree checks the routes the API server exposes when it hosts
// the game files itself, and that a missing manifest gets generated.
func TestMountPatchTree(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, patchtree.FilesDir, "exe")
	if err := os.MkdirAll(exe, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exe, "mhf.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := NewTestConfig()
	cfg.API.PatchTree.Enabled = true
	cfg.API.PatchTree.Root = root
	s := NewAPIServer(&Config{Logger: NewTestLogger(t), ErupeConfig: cfg})

	r := mux.NewRouter()
	r.HandleFunc("/", s.LandingPage)
	s.mountPatchTree(r)

	// Manifest generation runs in the background; wait for it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, patchtree.ManifestFile)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("manifest was not generated")
		}
		time.Sleep(20 * time.Millisecond)
	}

	get := func(target string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))
		return rr
	}

	if rr := get("/mhf_file.php?chk=1"); rr.Code != http.StatusOK || rr.Body.String() != patchtree.CheckMessage {
		t.Errorf("chk: %d %q", rr.Code, rr.Body.String())
	}
	if rr := get("/mhf_file.php?key=1"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `exe\mhf.exe`) {
		t.Errorf("key: %d %q", rr.Code, rr.Body.String())
	}
	if rr := get("/mhfdat/exe/mhf.exe?id=3"); rr.Code != http.StatusOK || rr.Body.String() != "MZ" {
		t.Errorf("file: %d %q", rr.Code, rr.Body.String())
	}
	if rr := get("/mhfdat/../config.json"); rr.Code == http.StatusOK {
		t.Errorf("traversal served: %d", rr.Code)
	}
}

// TestPatchTreeDisabledRoutesAbsent makes sure nothing is mounted when the
// operator hosts files elsewhere (the default).
func TestPatchTreeDisabledRoutesAbsent(t *testing.T) {
	s := NewAPIServer(&Config{Logger: NewTestLogger(t), ErupeConfig: NewTestConfig()})
	r := newTestRouter(s)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/mhf_file.php?chk=1", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rr.Code)
	}
}
