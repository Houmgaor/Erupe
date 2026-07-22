package main

import (
	"fmt"
	"strings"

	"erupe-ce/network"
	"erupe-ce/network/pcap"
)

// maxPayloadDiffs is the maximum number of byte-level diffs to report per packet.
const maxPayloadDiffs = 16

// ByteDiff describes a single byte difference between expected and actual payloads.
type ByteDiff struct {
	Offset   int
	Expected byte
	Actual   byte
}

// PacketDiff describes a difference between an expected and actual packet.
type PacketDiff struct {
	Index          int
	Expected       pcap.PacketRecord
	Actual         *pcap.PacketRecord // nil if no response received
	OpcodeMismatch bool
	SizeDelta      int
	PayloadDiffs   []ByteDiff // byte-level diffs (when opcodes match and sizes match)
}

func (d PacketDiff) String() string {
	if d.Actual == nil {
		if d.Expected.Opcode == 0 {
			return fmt.Sprintf("#%d: unexpected extra response 0x%04X (%s)",
				d.Index, d.Expected.Opcode, network.PacketID(d.Expected.Opcode))
		}
		return fmt.Sprintf("#%d: expected 0x%04X (%s), got no response",
			d.Index, d.Expected.Opcode, network.PacketID(d.Expected.Opcode))
	}
	if d.OpcodeMismatch {
		return fmt.Sprintf("#%d: opcode mismatch: expected 0x%04X (%s), got 0x%04X (%s)",
			d.Index,
			d.Expected.Opcode, network.PacketID(d.Expected.Opcode),
			d.Actual.Opcode, network.PacketID(d.Actual.Opcode))
	}
	if d.SizeDelta != 0 {
		return fmt.Sprintf("#%d: 0x%04X (%s) size delta %+d bytes",
			d.Index, d.Expected.Opcode, network.PacketID(d.Expected.Opcode), d.SizeDelta)
	}
	if len(d.PayloadDiffs) > 0 {
		var sb strings.Builder
		fmt.Fprintf(&sb, "#%d: 0x%04X (%s) %d byte diff(s):",
			d.Index, d.Expected.Opcode, network.PacketID(d.Expected.Opcode), len(d.PayloadDiffs))
		for _, bd := range d.PayloadDiffs {
			fmt.Fprintf(&sb, " [0x%04X: %02X→%02X]", bd.Offset, bd.Expected, bd.Actual)
		}
		return sb.String()
	}
	return fmt.Sprintf("#%d: 0x%04X (%s) unknown diff",
		d.Index, d.Expected.Opcode, network.PacketID(d.Expected.Opcode))
}

// ComparePackets compares expected server responses against actual responses.
// Only compares S→C packets (server responses).
func ComparePackets(expected, actual []pcap.PacketRecord) []PacketDiff {
	expectedS2C := pcap.FilterByDirection(expected, pcap.DirServerToClient)
	actualS2C := pcap.FilterByDirection(actual, pcap.DirServerToClient)

	var diffs []PacketDiff
	for i, exp := range expectedS2C {
		if i >= len(actualS2C) {
			diffs = append(diffs, PacketDiff{
				Index:    i,
				Expected: exp,
				Actual:   nil,
			})
			continue
		}
		act := actualS2C[i]
		if exp.Opcode != act.Opcode {
			diffs = append(diffs, PacketDiff{
				Index:          i,
				Expected:       exp,
				Actual:         &act,
				OpcodeMismatch: true,
			})
		} else if len(exp.Payload) != len(act.Payload) {
			diffs = append(diffs, PacketDiff{
				Index:     i,
				Expected:  exp,
				Actual:    &act,
				SizeDelta: len(act.Payload) - len(exp.Payload),
			})
		} else {
			// Same opcode and size — check for byte-level diffs.
			byteDiffs := comparePayloads(exp.Payload, act.Payload)
			if len(byteDiffs) > 0 {
				diffs = append(diffs, PacketDiff{
					Index:        i,
					Expected:     exp,
					Actual:       &act,
					PayloadDiffs: byteDiffs,
				})
			}
		}
	}

	// Extra actual packets beyond expected.
	for i := len(expectedS2C); i < len(actualS2C); i++ {
		act := actualS2C[i]
		diffs = append(diffs, PacketDiff{
			Index:    i,
			Expected: pcap.PacketRecord{},
			Actual:   &act,
		})
	}

	return diffs
}

// comparePayloads returns byte-level diffs between two equal-length payloads.
// Returns at most maxPayloadDiffs entries.
func comparePayloads(expected, actual []byte) []ByteDiff {
	var diffs []ByteDiff
	for i := 0; i < len(expected) && len(diffs) < maxPayloadDiffs; i++ {
		if expected[i] != actual[i] {
			diffs = append(diffs, ByteDiff{
				Offset:   i,
				Expected: expected[i],
				Actual:   actual[i],
			})
		}
	}
	return diffs
}
