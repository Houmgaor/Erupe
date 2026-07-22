package pcap

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Reader reads .mhfr capture files.
type Reader struct {
	r      io.Reader
	Header FileHeader
	Meta   SessionMetadata
}

// NewReader creates a Reader, reading and validating the file header and metadata.
func NewReader(r io.Reader) (*Reader, error) {
	// Read magic.
	magicBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, magicBuf); err != nil {
		return nil, fmt.Errorf("pcap: read magic: %w", err)
	}
	if string(magicBuf) != Magic {
		return nil, fmt.Errorf("pcap: invalid magic %q, expected %q", string(magicBuf), Magic)
	}

	var hdr FileHeader

	if err := binary.Read(r, binary.BigEndian, &hdr.Version); err != nil {
		return nil, fmt.Errorf("pcap: read version: %w", err)
	}
	if hdr.Version != FormatVersion {
		return nil, fmt.Errorf("pcap: unsupported version %d, expected %d", hdr.Version, FormatVersion)
	}

	var serverType byte
	if err := binary.Read(r, binary.BigEndian, &serverType); err != nil {
		return nil, fmt.Errorf("pcap: read server type: %w", err)
	}
	hdr.ServerType = ServerType(serverType)

	if err := binary.Read(r, binary.BigEndian, &hdr.ClientMode); err != nil {
		return nil, fmt.Errorf("pcap: read client mode: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &hdr.SessionStartNs); err != nil {
		return nil, fmt.Errorf("pcap: read session start: %w", err)
	}

	// Skip 4 reserved bytes.
	if _, err := io.ReadFull(r, make([]byte, 4)); err != nil {
		return nil, fmt.Errorf("pcap: read reserved: %w", err)
	}

	if err := binary.Read(r, binary.BigEndian, &hdr.MetadataLen); err != nil {
		return nil, fmt.Errorf("pcap: read metadata len: %w", err)
	}

	// Skip 8 reserved bytes.
	if _, err := io.ReadFull(r, make([]byte, 8)); err != nil {
		return nil, fmt.Errorf("pcap: read reserved: %w", err)
	}

	// Read metadata JSON.
	metaBytes := make([]byte, hdr.MetadataLen)
	if _, err := io.ReadFull(r, metaBytes); err != nil {
		return nil, fmt.Errorf("pcap: read metadata: %w", err)
	}

	var meta SessionMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, fmt.Errorf("pcap: unmarshal metadata: %w", err)
	}

	return &Reader{r: r, Header: hdr, Meta: meta}, nil
}

// ReadPacket reads the next packet record. Returns io.EOF when no more packets.
func (rd *Reader) ReadPacket() (PacketRecord, error) {
	var rec PacketRecord

	if err := binary.Read(rd.r, binary.BigEndian, &rec.TimestampNs); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return rec, io.EOF
		}
		return rec, fmt.Errorf("pcap: read timestamp: %w", err)
	}

	var dir byte
	if err := binary.Read(rd.r, binary.BigEndian, &dir); err != nil {
		return rec, fmt.Errorf("pcap: read direction: %w", err)
	}
	rec.Direction = Direction(dir)

	if err := binary.Read(rd.r, binary.BigEndian, &rec.Opcode); err != nil {
		return rec, fmt.Errorf("pcap: read opcode: %w", err)
	}

	var payloadLen uint32
	if err := binary.Read(rd.r, binary.BigEndian, &payloadLen); err != nil {
		return rec, fmt.Errorf("pcap: read payload len: %w", err)
	}

	rec.Payload = make([]byte, payloadLen)
	if _, err := io.ReadFull(rd.r, rec.Payload); err != nil {
		return rec, fmt.Errorf("pcap: read payload: %w", err)
	}

	return rec, nil
}
