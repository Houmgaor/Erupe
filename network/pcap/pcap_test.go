package pcap

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	hdr := FileHeader{
		Version:        FormatVersion,
		ServerType:     ServerTypeChannel,
		ClientMode:     40, // ZZ
		SessionStartNs: 1700000000000000000,
	}
	meta := SessionMetadata{
		ServerVersion: "test-v1",
		Host:          "127.0.0.1",
		Port:          54001,
		CharID:        42,
		UserID:        7,
		RemoteAddr:    "192.168.1.100:12345",
	}

	w, err := NewWriter(&buf, hdr, meta)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	packets := []PacketRecord{
		{TimestampNs: 1700000000000000100, Direction: DirClientToServer, Opcode: 0x0013, Payload: []byte{0x00, 0x13, 0x01, 0x02}},
		{TimestampNs: 1700000000000000200, Direction: DirServerToClient, Opcode: 0x0012, Payload: []byte{0x00, 0x12, 0xAA, 0xBB, 0xCC}},
		{TimestampNs: 1700000000000000300, Direction: DirClientToServer, Opcode: 0x0061, Payload: []byte{0x00, 0x61}},
	}

	for _, p := range packets {
		if err := w.WritePacket(p); err != nil {
			t.Fatalf("WritePacket: %v", err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Read it back.
	r, err := NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	// Verify header.
	if r.Header.Version != FormatVersion {
		t.Errorf("Version = %d, want %d", r.Header.Version, FormatVersion)
	}
	if r.Header.ServerType != ServerTypeChannel {
		t.Errorf("ServerType = %d, want %d", r.Header.ServerType, ServerTypeChannel)
	}
	if r.Header.ClientMode != 40 {
		t.Errorf("ClientMode = %d, want 40", r.Header.ClientMode)
	}
	if r.Header.SessionStartNs != 1700000000000000000 {
		t.Errorf("SessionStartNs = %d, want 1700000000000000000", r.Header.SessionStartNs)
	}

	// Verify metadata.
	if r.Meta.ServerVersion != "test-v1" {
		t.Errorf("ServerVersion = %q, want %q", r.Meta.ServerVersion, "test-v1")
	}
	if r.Meta.CharID != 42 {
		t.Errorf("CharID = %d, want 42", r.Meta.CharID)
	}

	// Verify packets.
	for i, want := range packets {
		got, err := r.ReadPacket()
		if err != nil {
			t.Fatalf("ReadPacket[%d]: %v", i, err)
		}
		if got.TimestampNs != want.TimestampNs {
			t.Errorf("[%d] TimestampNs = %d, want %d", i, got.TimestampNs, want.TimestampNs)
		}
		if got.Direction != want.Direction {
			t.Errorf("[%d] Direction = %d, want %d", i, got.Direction, want.Direction)
		}
		if got.Opcode != want.Opcode {
			t.Errorf("[%d] Opcode = 0x%04X, want 0x%04X", i, got.Opcode, want.Opcode)
		}
		if !bytes.Equal(got.Payload, want.Payload) {
			t.Errorf("[%d] Payload = %v, want %v", i, got.Payload, want.Payload)
		}
	}

	// Verify EOF.
	_, err = r.ReadPacket()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestEmptyCapture(t *testing.T) {
	var buf bytes.Buffer

	hdr := FileHeader{
		Version:        FormatVersion,
		ServerType:     ServerTypeSign,
		ClientMode:     40,
		SessionStartNs: 1000,
	}
	meta := SessionMetadata{}

	w, err := NewWriter(&buf, hdr, meta)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	_, err = r.ReadPacket()
	if err != io.EOF {
		t.Errorf("expected io.EOF for empty capture, got %v", err)
	}
	_ = r // use reader
}

func TestInvalidMagic(t *testing.T) {
	data := []byte("NOPE" + "\x00\x01\x03\x28" + "\x00\x00\x00\x00\x00\x00\x00\x01" + "\x00\x00\x00\x00" + "\x00\x00\x00\x02" + "\x00\x00\x00\x00\x00\x00\x00\x00" + "{}")
	_, err := NewReader(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for invalid magic")
	}
}

func TestInvalidVersion(t *testing.T) {
	// Valid magic, bad version (99).
	var buf bytes.Buffer
	buf.WriteString(Magic)
	buf.Write([]byte{0x00, 0x63}) // version 99
	buf.Write(make([]byte, 26))   // rest of header
	_, err := NewReader(&buf)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestLargePayload(t *testing.T) {
	var buf bytes.Buffer

	hdr := FileHeader{
		Version:        FormatVersion,
		ServerType:     ServerTypeChannel,
		ClientMode:     40,
		SessionStartNs: 1000,
	}
	meta := SessionMetadata{}

	w, err := NewWriter(&buf, hdr, meta)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	// 64KB payload.
	payload := make([]byte, 65536)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	rec := PacketRecord{
		TimestampNs: 2000,
		Direction:   DirServerToClient,
		Opcode:      0xFFFF,
		Payload:     payload,
	}
	if err := w.WritePacket(rec); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	got, err := r.ReadPacket()
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if len(got.Payload) != 65536 {
		t.Errorf("payload len = %d, want 65536", len(got.Payload))
	}
	if !bytes.Equal(got.Payload, payload) {
		t.Error("payload mismatch")
	}
}

func TestFilterByOpcode(t *testing.T) {
	records := []PacketRecord{
		{Opcode: 0x01},
		{Opcode: 0x02},
		{Opcode: 0x03},
		{Opcode: 0x01},
	}
	got := FilterByOpcode(records, 0x01, 0x03)
	if len(got) != 3 {
		t.Errorf("FilterByOpcode: got %d records, want 3", len(got))
	}
}

func TestFilterByDirection(t *testing.T) {
	records := []PacketRecord{
		{Direction: DirClientToServer},
		{Direction: DirServerToClient},
		{Direction: DirClientToServer},
	}
	got := FilterByDirection(records, DirServerToClient)
	if len(got) != 1 {
		t.Errorf("FilterByDirection: got %d records, want 1", len(got))
	}
}

func TestFilterExcludeOpcodes(t *testing.T) {
	records := []PacketRecord{
		{Opcode: 0x10}, // MSG_SYS_END
		{Opcode: 0x11}, // MSG_SYS_NOP
		{Opcode: 0x61}, // something else
	}
	got := FilterExcludeOpcodes(records, 0x10, 0x11)
	if len(got) != 1 {
		t.Errorf("FilterExcludeOpcodes: got %d records, want 1", len(got))
	}
	if got[0].Opcode != 0x61 {
		t.Errorf("remaining opcode = 0x%04X, want 0x0061", got[0].Opcode)
	}
}

func TestDirectionString(t *testing.T) {
	if DirClientToServer.String() != "C→S" {
		t.Errorf("DirClientToServer.String() = %q", DirClientToServer.String())
	}
	if DirServerToClient.String() != "S→C" {
		t.Errorf("DirServerToClient.String() = %q", DirServerToClient.String())
	}
	if Direction(0xFF).String() != "???" {
		t.Errorf("unknown direction = %q", Direction(0xFF).String())
	}
}

func TestMetadataPadding(t *testing.T) {
	var buf bytes.Buffer

	hdr := FileHeader{
		Version:        FormatVersion,
		ServerType:     ServerTypeChannel,
		ClientMode:     40,
		SessionStartNs: 1000,
	}
	meta := SessionMetadata{Host: "127.0.0.1"}

	_, err := NewWriter(&buf, hdr, meta)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	// The metadata block should be at least MinMetadataSize.
	data := buf.Bytes()
	if len(data) < HeaderSize+MinMetadataSize {
		t.Errorf("file size %d < HeaderSize+MinMetadataSize (%d)", len(data), HeaderSize+MinMetadataSize)
	}
}

func TestPatchMetadata(t *testing.T) {
	// Create a capture file with initial metadata.
	f, err := os.CreateTemp(t.TempDir(), "test-patch-*.mhfr")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()

	hdr := FileHeader{
		Version:        FormatVersion,
		ServerType:     ServerTypeChannel,
		ClientMode:     40,
		SessionStartNs: 1000,
	}
	meta := SessionMetadata{Host: "127.0.0.1", Port: 54001}

	w, err := NewWriter(f, hdr, meta)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	// Write a packet so we can verify it survives patching.
	if err := w.WritePacket(PacketRecord{
		TimestampNs: 2000, Direction: DirClientToServer, Opcode: 0x0013, Payload: []byte{0x00, 0x13},
	}); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Patch metadata with CharID/UserID.
	patched := SessionMetadata{
		Host:   "127.0.0.1",
		Port:   54001,
		CharID: 42,
		UserID: 7,
	}
	if err := PatchMetadata(f, patched); err != nil {
		t.Fatalf("PatchMetadata: %v", err)
	}

	// Re-read from the beginning.
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	r, err := NewReader(f)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	// Verify patched metadata.
	if r.Meta.CharID != 42 {
		t.Errorf("CharID = %d, want 42", r.Meta.CharID)
	}
	if r.Meta.UserID != 7 {
		t.Errorf("UserID = %d, want 7", r.Meta.UserID)
	}
	if r.Meta.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want %q", r.Meta.Host, "127.0.0.1")
	}

	// Verify packet survived.
	rec, err := r.ReadPacket()
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if rec.Opcode != 0x0013 {
		t.Errorf("Opcode = 0x%04X, want 0x0013", rec.Opcode)
	}
}

func TestServerTypeString(t *testing.T) {
	if ServerTypeSign.String() != "sign" {
		t.Errorf("ServerTypeSign.String() = %q", ServerTypeSign.String())
	}
	if ServerTypeEntrance.String() != "entrance" {
		t.Errorf("ServerTypeEntrance.String() = %q", ServerTypeEntrance.String())
	}
	if ServerTypeChannel.String() != "channel" {
		t.Errorf("ServerTypeChannel.String() = %q", ServerTypeChannel.String())
	}
	if ServerType(0xFF).String() != "unknown" {
		t.Errorf("unknown server type = %q", ServerType(0xFF).String())
	}
}
