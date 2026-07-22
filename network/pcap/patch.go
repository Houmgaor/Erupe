package pcap

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
)

// PatchMetadata rewrites the metadata block in a .mhfr capture file.
// The file must have been written with padded metadata (MinMetadataSize).
// The new JSON must fit within the existing MetadataLen allocation.
func PatchMetadata(f *os.File, meta SessionMetadata) error {
	newJSON, err := json.Marshal(&meta)
	if err != nil {
		return fmt.Errorf("pcap: marshal metadata: %w", err)
	}

	// Read MetadataLen from header (offset 20: after magic(4)+version(2)+servertype(1)+clientmode(1)+startnanos(8)+reserved(4)).
	var metaLen uint32
	if _, err := f.Seek(20, 0); err != nil {
		return fmt.Errorf("pcap: seek to metadata len: %w", err)
	}
	if err := binary.Read(f, binary.BigEndian, &metaLen); err != nil {
		return fmt.Errorf("pcap: read metadata len: %w", err)
	}

	if uint32(len(newJSON)) > metaLen {
		return fmt.Errorf("pcap: new metadata (%d bytes) exceeds allocated space (%d bytes)", len(newJSON), metaLen)
	}

	// Pad with spaces to fill the allocated block.
	padded := make([]byte, metaLen)
	copy(padded, newJSON)
	for i := len(newJSON); i < len(padded); i++ {
		padded[i] = ' '
	}

	// Write at offset HeaderSize (32).
	if _, err := f.Seek(HeaderSize, 0); err != nil {
		return fmt.Errorf("pcap: seek to metadata: %w", err)
	}
	if _, err := f.Write(padded); err != nil {
		return fmt.Errorf("pcap: write metadata: %w", err)
	}

	return nil
}
