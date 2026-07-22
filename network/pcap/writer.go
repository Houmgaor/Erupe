package pcap

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Writer writes .mhfr capture files.
type Writer struct {
	bw *bufio.Writer
}

// NewWriter creates a Writer, immediately writing the file header and metadata block.
func NewWriter(w io.Writer, header FileHeader, meta SessionMetadata) (*Writer, error) {
	metaBytes, err := json.Marshal(&meta)
	if err != nil {
		return nil, fmt.Errorf("pcap: marshal metadata: %w", err)
	}
	// Pad metadata to MinMetadataSize so PatchMetadata can update it in-place.
	if len(metaBytes) < MinMetadataSize {
		padded := make([]byte, MinMetadataSize)
		copy(padded, metaBytes)
		for i := len(metaBytes); i < MinMetadataSize; i++ {
			padded[i] = ' '
		}
		metaBytes = padded
	}
	header.MetadataLen = uint32(len(metaBytes))

	bw := bufio.NewWriter(w)

	// Write 32-byte file header.
	if _, err := bw.WriteString(Magic); err != nil {
		return nil, err
	}
	if err := binary.Write(bw, binary.BigEndian, header.Version); err != nil {
		return nil, err
	}
	if err := bw.WriteByte(byte(header.ServerType)); err != nil {
		return nil, err
	}
	if err := bw.WriteByte(header.ClientMode); err != nil {
		return nil, err
	}
	if err := binary.Write(bw, binary.BigEndian, header.SessionStartNs); err != nil {
		return nil, err
	}
	// 4 bytes reserved
	if _, err := bw.Write(make([]byte, 4)); err != nil {
		return nil, err
	}
	if err := binary.Write(bw, binary.BigEndian, header.MetadataLen); err != nil {
		return nil, err
	}
	// 8 bytes reserved
	if _, err := bw.Write(make([]byte, 8)); err != nil {
		return nil, err
	}

	// Write metadata JSON block.
	if _, err := bw.Write(metaBytes); err != nil {
		return nil, err
	}

	if err := bw.Flush(); err != nil {
		return nil, err
	}

	return &Writer{bw: bw}, nil
}

// WritePacket appends a single packet record.
func (w *Writer) WritePacket(rec PacketRecord) error {
	if err := binary.Write(w.bw, binary.BigEndian, rec.TimestampNs); err != nil {
		return err
	}
	if err := w.bw.WriteByte(byte(rec.Direction)); err != nil {
		return err
	}
	if err := binary.Write(w.bw, binary.BigEndian, rec.Opcode); err != nil {
		return err
	}
	if err := binary.Write(w.bw, binary.BigEndian, uint32(len(rec.Payload))); err != nil {
		return err
	}
	if _, err := w.bw.Write(rec.Payload); err != nil {
		return err
	}
	return nil
}

// Flush flushes the buffered writer.
func (w *Writer) Flush() error {
	return w.bw.Flush()
}
