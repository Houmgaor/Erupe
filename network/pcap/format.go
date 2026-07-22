package pcap

import "encoding/json"

// Capture file format constants.
const (
	// Magic is the 4-byte magic number for .mhfr capture files.
	Magic = "MHFR"

	// FormatVersion is the current capture format version.
	FormatVersion uint16 = 1

	// HeaderSize is the fixed size of the file header in bytes.
	HeaderSize = 32

	// MinMetadataSize is the minimum metadata block size in bytes.
	// Metadata is padded to at least this size to allow in-place patching
	// (e.g., adding CharID/UserID after login).
	MinMetadataSize = 512
)

// Direction indicates whether a packet was sent or received.
type Direction byte

const (
	DirClientToServer Direction = 0x01
	DirServerToClient Direction = 0x02
)

func (d Direction) String() string {
	switch d {
	case DirClientToServer:
		return "C→S"
	case DirServerToClient:
		return "S→C"
	default:
		return "???"
	}
}

// ServerType identifies which server a capture originated from.
type ServerType byte

const (
	ServerTypeSign     ServerType = 0x01
	ServerTypeEntrance ServerType = 0x02
	ServerTypeChannel  ServerType = 0x03
)

func (st ServerType) String() string {
	switch st {
	case ServerTypeSign:
		return "sign"
	case ServerTypeEntrance:
		return "entrance"
	case ServerTypeChannel:
		return "channel"
	default:
		return "unknown"
	}
}

// FileHeader is the fixed 32-byte header at the start of a .mhfr file.
//
//	[4B] Magic "MHFR"
//	[2B] Version
//	[1B] ServerType
//	[1B] ClientMode
//	[8B] SessionStartNs
//	[4B] Reserved
//	[4B] MetadataLen
//	[8B] Reserved
type FileHeader struct {
	Version        uint16
	ServerType     ServerType
	ClientMode     byte
	SessionStartNs int64
	MetadataLen    uint32
}

// SessionMetadata is the JSON-encoded metadata block following the file header.
type SessionMetadata struct {
	ServerVersion string `json:"server_version,omitempty"`
	Host          string `json:"host,omitempty"`
	Port          int    `json:"port,omitempty"`
	CharID        uint32 `json:"char_id,omitempty"`
	UserID        uint32 `json:"user_id,omitempty"`
	RemoteAddr    string `json:"remote_addr,omitempty"`
}

// MarshalJSON serializes the metadata to JSON.
func (m *SessionMetadata) MarshalJSON() ([]byte, error) {
	type Alias SessionMetadata
	return json.Marshal((*Alias)(m))
}

// PacketRecord is a single captured packet.
//
//	[8B] TimestampNs  [1B] Direction  [2B] Opcode  [4B] PayloadLen  [NB] Payload
type PacketRecord struct {
	TimestampNs int64
	Direction   Direction
	Opcode      uint16
	Payload     []byte // Full decrypted packet bytes (includes the 2-byte opcode prefix)
}

// PacketRecordHeaderSize is the fixed overhead per packet record (before payload).
const PacketRecordHeaderSize = 8 + 1 + 2 + 4 // 15 bytes
