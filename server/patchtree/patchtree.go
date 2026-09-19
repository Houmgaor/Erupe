// Package patchtree serves a game-file tree in the format the MHF launcher
// (mhl.dll) and mhf-outpost sync from, so an operator can host the client
// files from the Erupe binary instead of a separate web server.
//
// The tree lives under a root directory:
//
//	<root>/key.txt         manifest, one line per file (generated here)
//	<root>/mhfdat/exe/…    files that live next to mhf.exe
//	<root>/mhfdat/dat/…    the game's dat/ folder
//
// and is served as
//
//	GET /mhf_file.php?key=<n>             the manifest
//	GET /mhf_file.php?chk=<n>             "[mhf Check Message:0]"
//	GET /mhfdat/<exe|dat>/<file>?id=<n>   one file
//
// which are the request shapes recovered from mhl.dll. <n> is a cache-buster.
// Everything must be reachable over plain HTTP: the launcher does not do TLS.
package patchtree

import (
	"bufio"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// ManifestFile is the manifest's name under the tree root. The launcher
	// saves whatever it receives as MHFUP_00.DAT; the name here is the one
	// Mezeporta/Servers' genMhfKey.py used, kept so existing trees work as-is.
	ManifestFile = "key.txt"
	// FilesDir is the directory under the root that holds exe/ and dat/.
	FilesDir = "mhfdat"
	// CheckMessage is the fixed reply to the launcher's connectivity probe.
	CheckMessage = "[mhf Check Message:0]"

	// filetimeEpochDiff is the number of 100 ns ticks between 1601-01-01
	// (Windows FILETIME epoch) and 1970-01-01 (Unix epoch).
	filetimeEpochDiff = 116444736000000000
)

// Entry is one manifest line.
type Entry struct {
	CRC32    uint32
	Filetime uint64 // Windows FILETIME of the last modification
	Path     string // Relative to the game folder, backslash-separated (exe\mhf.exe, dat\…)
	Size     int64
}

// String renders the entry as the launcher expects it:
// CRC32,FILETIME_LO,FILETIME_HI,path,size,0
func (e Entry) String() string {
	return fmt.Sprintf("%08X,%08X,%08X,%s,%d,0",
		e.CRC32, uint32(e.Filetime), uint32(e.Filetime>>32), e.Path, e.Size)
}

// Scan walks <root>/mhfdat and computes a manifest entry for every regular
// file. Zero-byte files are skipped: the launcher treats a zero size as
// "nothing to fetch" and would loop on them. Entries are sorted by path so
// the output is deterministic.
func Scan(root string) ([]Entry, error) {
	base := filepath.Join(root, FilesDir)
	info, err := os.Stat(base)
	if err != nil {
		return nil, fmt.Errorf("patch tree: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("patch tree: %s is not a directory", base)
	}

	var entries []Entry
	err = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() || fi.Size() == 0 {
			return nil
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		sum, err := fileCRC32(p)
		if err != nil {
			return err
		}
		entries = append(entries, Entry{
			CRC32:    sum,
			Filetime: toFiletime(fi.ModTime()),
			Path:     strings.ReplaceAll(filepath.ToSlash(rel), "/", `\`),
			Size:     fi.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("patch tree: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// WriteManifest writes entries in manifest format.
func WriteManifest(w io.Writer, entries []Entry) error {
	bw := bufio.NewWriter(w)
	for _, e := range entries {
		if _, err := bw.WriteString(e.String() + "\n"); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// Generate scans the tree and atomically replaces <root>/key.txt. It returns
// the number of entries written.
func Generate(root string) (int, error) {
	entries, err := Scan(root)
	if err != nil {
		return 0, err
	}
	dst := filepath.Join(root, ManifestFile)
	tmp, err := os.CreateTemp(root, ManifestFile+".*.tmp")
	if err != nil {
		return 0, fmt.Errorf("patch tree: %w", err)
	}
	tmpName := tmp.Name()
	if err := WriteManifest(tmp, entries); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return 0, fmt.Errorf("patch tree: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return 0, fmt.Errorf("patch tree: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return 0, fmt.Errorf("patch tree: %w", err)
	}
	return len(entries), nil
}

// Stale reports whether the manifest is missing or older than any file under
// <root>/mhfdat. It only stats files, so it is cheap enough to run at every
// startup; Generate, which reads every byte, is not.
func Stale(root string) (bool, error) {
	mi, err := os.Stat(filepath.Join(root, ManifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	manifestTime := mi.ModTime()

	errStale := errors.New("stale")
	err = filepath.WalkDir(filepath.Join(root, FilesDir), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if fi.ModTime().After(manifestTime) {
			return errStale
		}
		return nil
	})
	if errors.Is(err, errStale) {
		return true, nil
	}
	return false, err
}

// ManifestHandler serves /mhf_file.php: the manifest for ?key=, the check
// message for ?chk= / ?chksha=, 404 otherwise.
func ManifestHandler(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		switch {
		case q.Has("key"):
			f, err := os.Open(filepath.Join(root, ManifestFile))
			if errors.Is(err, fs.ErrNotExist) {
				http.Error(w, "manifest not generated yet", http.StatusServiceUnavailable)
				return
			}
			if err != nil {
				http.Error(w, "manifest unavailable", http.StatusInternalServerError)
				return
			}
			defer func() { _ = f.Close() }()
			fi, err := f.Stat()
			if err != nil {
				http.Error(w, "manifest unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", `inline; filename="MHFUP_00.DAT"`)
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeContent(w, r, ManifestFile, fi.ModTime(), f)
		case q.Has("chk"), q.Has("chksha"):
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, CheckMessage)
		default:
			http.NotFound(w, r)
		}
	})
}

// FilesHandler serves /mhfdat/<exe|dat>/<file> from <root>/mhfdat. Only
// regular files are served (no directory listings), always as
// application/octet-stream, with Range support so interrupted downloads can
// resume. The handler expects the request path to still carry the /mhfdat/
// prefix, i.e. mount it with a PathPrefix, not StripPrefix.
func FilesHandler(root string) http.Handler {
	base := filepath.Join(root, FilesDir)
	prefix := "/" + FilesDir + "/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, prefix)
		// The launcher only ever asks for exe\… and dat\…; anything else in
		// the tree (or outside it) is not for serving.
		clean := filepath.Clean(filepath.FromSlash(rel))
		sep := string(filepath.Separator)
		allowed := strings.HasPrefix(clean, "exe"+sep) || strings.HasPrefix(clean, "dat"+sep)
		if rel == "" || !allowed || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			http.NotFound(w, r)
			return
		}
		full := filepath.Join(base, clean)
		f, err := os.Open(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = f.Close() }()
		fi, err := f.Stat()
		if err != nil || !fi.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, "", fi.ModTime(), f)
	})
}

func fileCRC32(p string) (uint32, error) {
	f, err := os.Open(p)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return 0, err
	}
	return h.Sum32(), nil
}

func toFiletime(t time.Time) uint64 {
	return uint64(t.UnixNano()/100) + filetimeEpochDiff
}
