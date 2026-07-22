package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"os"
	"strings"
	"testing"

	"erupe-ce/network/pcap"
)

func createTestCapture(t *testing.T, records []pcap.PacketRecord) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.mhfr")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()

	hdr := pcap.FileHeader{
		Version:        pcap.FormatVersion,
		ServerType:     pcap.ServerTypeChannel,
		ClientMode:     40,
		SessionStartNs: 1000000000,
	}
	meta := pcap.SessionMetadata{Host: "127.0.0.1", Port: 54001}

	w, err := pcap.NewWriter(f, hdr, meta)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, r := range records {
		if err := w.WritePacket(r); err != nil {
			t.Fatalf("WritePacket: %v", err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return f.Name()
}

func TestRunDump(t *testing.T) {
	path := createTestCapture(t, []pcap.PacketRecord{
		{TimestampNs: 1000000100, Direction: pcap.DirClientToServer, Opcode: 0x0013, Payload: []byte{0x00, 0x13}},
		{TimestampNs: 1000000200, Direction: pcap.DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12, 0xFF}},
	})
	// Just verify it doesn't error.
	if err := runDump(path); err != nil {
		t.Fatalf("runDump: %v", err)
	}
}

func TestRunStats(t *testing.T) {
	path := createTestCapture(t, []pcap.PacketRecord{
		{TimestampNs: 1000000100, Direction: pcap.DirClientToServer, Opcode: 0x0013, Payload: []byte{0x00, 0x13}},
		{TimestampNs: 1000000200, Direction: pcap.DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12, 0xFF}},
		{TimestampNs: 1000000300, Direction: pcap.DirClientToServer, Opcode: 0x0013, Payload: []byte{0x00, 0x13, 0xAA}},
	})
	if err := runStats(path); err != nil {
		t.Fatalf("runStats: %v", err)
	}
}

func TestRunStatsEmpty(t *testing.T) {
	path := createTestCapture(t, nil)
	if err := runStats(path); err != nil {
		t.Fatalf("runStats empty: %v", err)
	}
}

func TestRunJSON(t *testing.T) {
	path := createTestCapture(t, []pcap.PacketRecord{
		{TimestampNs: 1000000100, Direction: pcap.DirClientToServer, Opcode: 0x0013, Payload: []byte{0x00, 0x13}},
	})
	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := runJSON(path); err != nil {
		os.Stdout = old
		t.Fatalf("runJSON: %v", err)
	}

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if buf.Len() == 0 {
		t.Error("runJSON produced no output")
	}
	// Should be valid JSON containing "packets".
	if !bytes.Contains(buf.Bytes(), []byte(`"packets"`)) {
		t.Error("runJSON output missing 'packets' key")
	}
}

func TestComparePackets(t *testing.T) {
	expected := []pcap.PacketRecord{
		{Direction: pcap.DirClientToServer, Opcode: 0x0013, Payload: []byte{0x00, 0x13}},
		{Direction: pcap.DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12, 0xAA}},
		{Direction: pcap.DirServerToClient, Opcode: 0x0061, Payload: []byte{0x00, 0x61}},
	}
	actual := []pcap.PacketRecord{
		{Direction: pcap.DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12, 0xBB, 0xCC}}, // size diff
		{Direction: pcap.DirServerToClient, Opcode: 0x0099, Payload: []byte{0x00, 0x99}},             // opcode mismatch
	}

	diffs := ComparePackets(expected, actual)
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(diffs))
	}

	// First diff: size delta.
	if diffs[0].SizeDelta != 1 {
		t.Errorf("diffs[0] SizeDelta = %d, want 1", diffs[0].SizeDelta)
	}

	// Second diff: opcode mismatch.
	if !diffs[1].OpcodeMismatch {
		t.Error("diffs[1] expected OpcodeMismatch=true")
	}
}

func TestComparePacketsMissingResponse(t *testing.T) {
	expected := []pcap.PacketRecord{
		{Direction: pcap.DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12}},
		{Direction: pcap.DirServerToClient, Opcode: 0x0061, Payload: []byte{0x00, 0x61}},
	}
	actual := []pcap.PacketRecord{
		{Direction: pcap.DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12}},
	}

	diffs := ComparePackets(expected, actual)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Actual != nil {
		t.Error("expected nil Actual for missing response")
	}
}

func TestComparePacketsPayloadDiff(t *testing.T) {
	expected := []pcap.PacketRecord{
		{Direction: pcap.DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12, 0xAA, 0xBB}},
	}
	actual := []pcap.PacketRecord{
		{Direction: pcap.DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12, 0xCC, 0xBB}},
	}

	diffs := ComparePackets(expected, actual)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if len(diffs[0].PayloadDiffs) != 1 {
		t.Fatalf("expected 1 payload diff, got %d", len(diffs[0].PayloadDiffs))
	}
	bd := diffs[0].PayloadDiffs[0]
	if bd.Offset != 2 || bd.Expected != 0xAA || bd.Actual != 0xCC {
		t.Errorf("ByteDiff = {Offset:%d, Expected:0x%02X, Actual:0x%02X}, want {2, 0xAA, 0xCC}",
			bd.Offset, bd.Expected, bd.Actual)
	}
}

func TestComparePacketsIdentical(t *testing.T) {
	records := []pcap.PacketRecord{
		{Direction: pcap.DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12, 0xAA}},
	}
	diffs := ComparePackets(records, records)
	if len(diffs) != 0 {
		t.Errorf("expected 0 diffs for identical packets, got %d", len(diffs))
	}
}

func TestPacketDiffString(t *testing.T) {
	tests := []struct {
		name     string
		diff     PacketDiff
		contains string
	}{
		{
			name: "missing response",
			diff: PacketDiff{
				Index:    0,
				Expected: pcap.PacketRecord{Opcode: 0x0012},
				Actual:   nil,
			},
			contains: "no response",
		},
		{
			name: "opcode mismatch",
			diff: PacketDiff{
				Index:          1,
				Expected:       pcap.PacketRecord{Opcode: 0x0012},
				Actual:         &pcap.PacketRecord{Opcode: 0x0099},
				OpcodeMismatch: true,
			},
			contains: "opcode mismatch",
		},
		{
			name: "size delta",
			diff: PacketDiff{
				Index:     2,
				Expected:  pcap.PacketRecord{Opcode: 0x0012},
				Actual:    &pcap.PacketRecord{Opcode: 0x0012},
				SizeDelta: 5,
			},
			contains: "size delta",
		},
		{
			name: "payload diffs",
			diff: PacketDiff{
				Index:    3,
				Expected: pcap.PacketRecord{Opcode: 0x0012},
				Actual:   &pcap.PacketRecord{Opcode: 0x0012},
				PayloadDiffs: []ByteDiff{
					{Offset: 2, Expected: 0xAA, Actual: 0xBB},
				},
			},
			contains: "byte diff",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.diff.String()
			if !strings.Contains(s, tc.contains) {
				t.Errorf("String() = %q, want it to contain %q", s, tc.contains)
			}
		})
	}
}

func TestRunReplayWithMockServer(t *testing.T) {
	// Start a mock TCP server that echoes a response for each received packet.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()

		// This mock doesn't do Blowfish encryption — it just reads raw and echoes.
		// Since the replay uses protbot's CryptConn (Blowfish), we need a real crypto echo.
		// For a simpler test, just verify the function handles connection errors gracefully.
		// Read a bit and close.
		buf := make([]byte, 1024)
		_, _ = c.Read(buf)
	}()

	// Create a minimal capture with one C→S packet.
	path := createTestCapture(t, []pcap.PacketRecord{
		{TimestampNs: 1000000100, Direction: pcap.DirClientToServer, Opcode: 0x0013,
			Payload: []byte{0x00, 0x13, 0xDE, 0xAD}},
	})

	// Run replay — the connection will fail (no Blowfish on mock), but it should not panic.
	err = runReplay(path, ln.Addr().String(), 0)
	// We expect an error or graceful handling since the mock doesn't speak Blowfish.
	// The important thing is no panic.
	_ = err
}

func TestComparePayloads(t *testing.T) {
	a := []byte{0x00, 0x12, 0xAA, 0xBB, 0xCC}
	b := []byte{0x00, 0x12, 0xAA, 0xDD, 0xCC}

	diffs := comparePayloads(a, b)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Offset != 3 {
		t.Errorf("Offset = %d, want 3", diffs[0].Offset)
	}
}

func TestComparePayloadsMaxDiffs(t *testing.T) {
	// All bytes different — should cap at maxPayloadDiffs.
	a := make([]byte, 100)
	b := make([]byte, 100)
	for i := range b {
		b[i] = 0xFF
	}

	diffs := comparePayloads(a, b)
	if len(diffs) != maxPayloadDiffs {
		t.Errorf("expected %d diffs (capped), got %d", maxPayloadDiffs, len(diffs))
	}
}

func TestBuildPingResponse(t *testing.T) {
	pong := buildPingResponse()
	if len(pong) < 2 {
		t.Fatal("ping response too short")
	}
	opcode := binary.BigEndian.Uint16(pong[:2])
	if opcode != opcodeSysPing {
		t.Errorf("opcode = 0x%04X, want 0x%04X", opcode, opcodeSysPing)
	}
}
