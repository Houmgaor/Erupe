package main

import (
	"encoding/binary"
	"strings"
	"testing"

	"erupe-ce/common/decryption"
)

// container builds a scenario container from chunk payloads; a nil chunk2
// omits its size field.
func container(c0, c1, c2 []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(c0)))
	out = binary.BigEndian.AppendUint32(out, uint32(len(c1)))
	out = append(out, c0...)
	out = append(out, c1...)
	if c2 != nil {
		out = binary.BigEndian.AppendUint32(out, uint32(len(c2)))
		out = append(out, c2...)
	}
	return out
}

func TestDiffScenarioChunks(t *testing.T) {
	dialog := []byte(strings.Repeat("@RETURN dialog line\x00", 20))
	for _, tt := range []struct {
		name       string
		orig, rec  []byte
		wantPrefix string
	}{
		{"identical", container([]byte("abc"), dialog, nil), container([]byte("abc"), dialog, nil), ""},
		{"same content, recompressed", container(nil, decryption.PackSimple(dialog), nil), container(nil, dialog, nil), ""},
		{"empty chunk2 vs absent", container([]byte("a"), nil, []byte{}), container([]byte("a"), nil, nil), ""},
		{"chunk0 shorter", container([]byte("abc\x00\xff\xff"), nil, nil), container([]byte("abc\x00\xff"), nil, nil), "chunk0 differs (5 vs 6"},
		{"chunk1 truncated", container(nil, decryption.PackSimple(dialog), nil), container(nil, dialog[:100], nil), "chunk1 differs (100 vs 400"},
		{"chunk2 differs", container(nil, nil, []byte("x")), container(nil, nil, []byte("y")), "chunk2 differs"},
		{"recompiled overruns", container([]byte("a"), nil, nil), []byte{0, 0, 0, 9, 0, 0, 0, 0}, "recompiled: chunk sizes"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := diffScenarioChunks(tt.orig, tt.rec)
			if tt.wantPrefix == "" && got != "" || !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("diffScenarioChunks() = %q, want prefix %q", got, tt.wantPrefix)
			}
		})
	}
}
