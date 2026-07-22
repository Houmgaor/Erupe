package pcap

// FilterByOpcode returns only records matching any of the given opcodes.
func FilterByOpcode(records []PacketRecord, opcodes ...uint16) []PacketRecord {
	set := make(map[uint16]struct{}, len(opcodes))
	for _, op := range opcodes {
		set[op] = struct{}{}
	}
	var out []PacketRecord
	for _, r := range records {
		if _, ok := set[r.Opcode]; ok {
			out = append(out, r)
		}
	}
	return out
}

// FilterByDirection returns only records matching the given direction.
func FilterByDirection(records []PacketRecord, dir Direction) []PacketRecord {
	var out []PacketRecord
	for _, r := range records {
		if r.Direction == dir {
			out = append(out, r)
		}
	}
	return out
}

// FilterExcludeOpcodes returns records excluding any of the given opcodes.
func FilterExcludeOpcodes(records []PacketRecord, opcodes ...uint16) []PacketRecord {
	set := make(map[uint16]struct{}, len(opcodes))
	for _, op := range opcodes {
		set[op] = struct{}{}
	}
	var out []PacketRecord
	for _, r := range records {
		if _, ok := set[r.Opcode]; !ok {
			out = append(out, r)
		}
	}
	return out
}
