package pcap

import (
	"encoding/binary"
	"os"
	"sync"
	"time"

	"erupe-ce/network"
)

// RecordingConn wraps a network.Conn and records all packets to a Writer.
// It is safe for concurrent use from separate send/recv goroutines.
type RecordingConn struct {
	inner          network.Conn
	writer         *Writer
	startNs        int64
	excludeOpcodes map[uint16]struct{}
	metaFile       *os.File         // capture file handle for metadata patching
	meta           *SessionMetadata // current metadata (mutated by SetSessionInfo)
	mu             sync.Mutex
}

// NewRecordingConn wraps inner, recording all packets to w.
// startNs is the session start time in nanoseconds (used as the time base).
// excludeOpcodes is an optional list of opcodes to skip when recording.
func NewRecordingConn(inner network.Conn, w *Writer, startNs int64, excludeOpcodes []uint16) *RecordingConn {
	var excl map[uint16]struct{}
	if len(excludeOpcodes) > 0 {
		excl = make(map[uint16]struct{}, len(excludeOpcodes))
		for _, op := range excludeOpcodes {
			excl[op] = struct{}{}
		}
	}
	return &RecordingConn{
		inner:          inner,
		writer:         w,
		startNs:        startNs,
		excludeOpcodes: excl,
	}
}

// SetCaptureFile sets the file handle and metadata pointer for in-place metadata patching.
// Must be called before SetSessionInfo. Not required if metadata patching is not needed.
func (rc *RecordingConn) SetCaptureFile(f *os.File, meta *SessionMetadata) {
	rc.mu.Lock()
	rc.metaFile = f
	rc.meta = meta
	rc.mu.Unlock()
}

// SetSessionInfo updates the CharID and UserID in the capture file metadata.
// This is called after login when the session identity is known.
func (rc *RecordingConn) SetSessionInfo(charID, userID uint32) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.meta == nil || rc.metaFile == nil {
		return
	}

	rc.meta.CharID = charID
	rc.meta.UserID = userID

	// Best-effort patch — log errors are handled by the caller.
	_ = PatchMetadata(rc.metaFile, *rc.meta)
}

// ReadPacket reads from the inner connection and records the packet as client-to-server.
func (rc *RecordingConn) ReadPacket() ([]byte, error) {
	data, err := rc.inner.ReadPacket()
	if err != nil {
		return data, err
	}
	rc.record(DirClientToServer, data)
	return data, nil
}

// SendPacket sends via the inner connection and records the packet as server-to-client.
func (rc *RecordingConn) SendPacket(data []byte) error {
	err := rc.inner.SendPacket(data)
	if err != nil {
		return err
	}
	rc.record(DirServerToClient, data)
	return nil
}

func (rc *RecordingConn) record(dir Direction, data []byte) {
	var opcode uint16
	if len(data) >= 2 {
		opcode = binary.BigEndian.Uint16(data[:2])
	}

	if rc.excludeOpcodes != nil {
		if _, excluded := rc.excludeOpcodes[opcode]; excluded {
			return
		}
	}

	rec := PacketRecord{
		TimestampNs: time.Now().UnixNano(),
		Direction:   dir,
		Opcode:      opcode,
		Payload:     data,
	}

	rc.mu.Lock()
	_ = rc.writer.WritePacket(rec)
	rc.mu.Unlock()
}
