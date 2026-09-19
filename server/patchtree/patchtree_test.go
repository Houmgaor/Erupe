package patchtree

import (
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTree builds a minimal patch tree and returns its root.
func newTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"mhfdat/exe/mhf.exe":         "MZ-not-really",
		"mhfdat/exe/url.lst":         "members.example\n",
		"mhfdat/dat/mhfdat.bin":      strings.Repeat("d", 1000),
		"mhfdat/dat/stage/st001.pac": "stage",
		"mhfdat/dat/empty.bin":       "", // must be skipped
		"mhfdat/README.txt":          "not exe or dat, never served",
		"secret.txt":                 "outside mhfdat",
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScanFormat(t *testing.T) {
	root := newTree(t)
	entries, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by path, zero-byte file skipped, README.txt is still in the
	// manifest (Scan lists the tree; it is the file handler that restricts
	// what is served).
	want := []string{
		`README.txt`, `dat\mhfdat.bin`, `dat\stage\st001.pac`, `exe\mhf.exe`, `exe\url.lst`,
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, e := range entries {
		if e.Path != want[i] {
			t.Errorf("entry %d path = %q, want %q", i, e.Path, want[i])
		}
	}

	var exe Entry
	for _, e := range entries {
		if e.Path == `exe\mhf.exe` {
			exe = e
		}
	}
	if exe.Size != int64(len("MZ-not-really")) {
		t.Errorf("size = %d", exe.Size)
	}
	if exe.CRC32 != crc32.ChecksumIEEE([]byte("MZ-not-really")) {
		t.Errorf("crc mismatch")
	}
	line := exe.String()
	fields := strings.Split(line, ",")
	if len(fields) != 6 || fields[5] != "0" || fields[3] != `exe\mhf.exe` {
		t.Fatalf("bad manifest line %q", line)
	}
	for _, f := range fields[:3] {
		if len(f) != 8 || strings.ToUpper(f) != f {
			t.Errorf("field %q is not 8 uppercase hex digits", f)
		}
	}
}

func TestFiletimeEpoch(t *testing.T) {
	// 1970-01-01T00:00:00Z is 116444736000000000 ticks after the FILETIME epoch.
	if got := toFiletime(time.Unix(0, 0)); got != filetimeEpochDiff {
		t.Fatalf("toFiletime(unix 0) = %d", got)
	}
	// Round-trips through the split LO/HI representation genMhfKey.py used.
	e := Entry{Filetime: 0x01DA01F4B61F3F00}
	if !strings.HasPrefix(e.String(), "00000000,B61F3F00,01DA01F4,") {
		t.Fatalf("filetime split = %s", e.String())
	}
}

func TestGenerateAndStale(t *testing.T) {
	root := newTree(t)

	stale, err := Stale(root)
	if err != nil || !stale {
		t.Fatalf("missing manifest: stale=%v err=%v", stale, err)
	}

	n, err := Generate(root)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("generated %d entries, want 5", n)
	}
	data, err := os.ReadFile(filepath.Join(root, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(data), "\n"); lines != 5 {
		t.Fatalf("manifest has %d lines", lines)
	}
	if strings.Contains(string(data), "empty.bin") {
		t.Fatal("zero-byte file listed in manifest")
	}

	stale, err = Stale(root)
	if err != nil || stale {
		t.Fatalf("fresh manifest: stale=%v err=%v", stale, err)
	}

	// Touch a file into the future: manifest is stale again.
	p := filepath.Join(root, "mhfdat", "dat", "mhfdat.bin")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	stale, err = Stale(root)
	if err != nil || !stale {
		t.Fatalf("after touch: stale=%v err=%v", stale, err)
	}

	// No leftover temp files.
	matches, _ := filepath.Glob(filepath.Join(root, ManifestFile+".*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("temp files left behind: %v", matches)
	}
}

func TestManifestHandler(t *testing.T) {
	root := newTree(t)
	h := ManifestHandler(root)

	get := func(target string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))
		return rr
	}

	if rr := get("/mhf_file.php?key=1"); rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("no manifest yet: %d", rr.Code)
	}
	if _, err := Generate(root); err != nil {
		t.Fatal(err)
	}

	rr := get("/mhf_file.php?key=42")
	if rr.Code != http.StatusOK {
		t.Fatalf("key: %d %s", rr.Code, rr.Body.String())
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "MHFUP_00.DAT") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if !strings.Contains(rr.Body.String(), `exe\mhf.exe`) {
		t.Errorf("manifest body missing entry: %s", rr.Body.String())
	}

	for _, target := range []string{"/mhf_file.php?chk=1", "/mhf_file.php?chksha=1"} {
		rr := get(target)
		if rr.Code != http.StatusOK || rr.Body.String() != CheckMessage {
			t.Errorf("%s: %d %q", target, rr.Code, rr.Body.String())
		}
	}

	if rr := get("/mhf_file.php?other=1"); rr.Code != http.StatusNotFound {
		t.Errorf("unknown query: %d", rr.Code)
	}
	if rr := get("/mhf_file.php"); rr.Code != http.StatusNotFound {
		t.Errorf("no query: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mhf_file.php?key=1", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rr.Code)
	}
}

func TestFilesHandler(t *testing.T) {
	root := newTree(t)
	h := FilesHandler(root)

	get := func(target string, hdr ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	rr := get("/mhfdat/exe/mhf.exe?id=7")
	if rr.Code != http.StatusOK {
		t.Fatalf("exe: %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	if body, _ := io.ReadAll(rr.Body); string(body) != "MZ-not-really" {
		t.Errorf("body = %q", body)
	}

	if rr := get("/mhfdat/dat/stage/st001.pac"); rr.Code != http.StatusOK {
		t.Errorf("nested dat: %d", rr.Code)
	}

	// Range requests are honoured so interrupted downloads can resume.
	rr = get("/mhfdat/dat/mhfdat.bin", "Range", "bytes=990-")
	if rr.Code != http.StatusPartialContent || rr.Body.Len() != 10 {
		t.Errorf("range: %d len=%d", rr.Code, rr.Body.Len())
	}

	denied := []string{
		"/mhfdat/README.txt",    // in the tree, but not under exe/ or dat/
		"/mhfdat/",              // directory
		"/mhfdat/exe/",          // directory
		"/mhfdat/dat/stage",     // directory
		"/mhfdat/../secret.txt", // traversal
		"/mhfdat/exe/../../secret.txt",
		"/mhfdat/exe/%2e%2e/%2e%2e/secret.txt",
		"/mhfdat/dat/missing.bin",
		"/other/exe/mhf.exe",
	}
	for _, target := range denied {
		if rr := get(target); rr.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", target, rr.Code)
		}
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mhfdat/exe/mhf.exe", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rr.Code)
	}
}

func TestScanMissingTree(t *testing.T) {
	if _, err := Scan(t.TempDir()); err == nil {
		t.Fatal("expected error for a root without mhfdat/")
	}
}
