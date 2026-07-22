// replay is a CLI tool for inspecting and replaying .mhfr packet capture files.
//
// Usage:
//
//	replay --capture file.mhfr --mode dump     # Human-readable text output
//	replay --capture file.mhfr --mode json     # JSON export
//	replay --capture file.mhfr --mode stats    # Opcode histogram, duration, counts
//	replay --capture file.mhfr --mode replay --target 127.0.0.1:54001 --no-auth  # Replay against live server
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"erupe-ce/cmd/protbot/conn"
	"erupe-ce/network"
	"erupe-ce/network/pcap"
)

// MSG_SYS_PING opcode for auto-responding to server pings.
const opcodeSysPing = 0x0017

func main() {
	capturePath := flag.String("capture", "", "Path to .mhfr capture file (required)")
	mode := flag.String("mode", "dump", "Mode: dump, json, stats, replay")
	target := flag.String("target", "", "Target server address for replay mode (host:port)")
	speed := flag.Float64("speed", 1.0, "Replay speed multiplier (e.g. 2.0 = 2x faster)")
	noAuth := flag.Bool("no-auth", false, "Skip auth token patching (requires DisableTokenCheck on server)")
	_ = noAuth // currently only no-auth mode is supported
	flag.Parse()

	if *capturePath == "" {
		fmt.Fprintln(os.Stderr, "error: --capture is required")
		flag.Usage()
		os.Exit(1)
	}

	switch *mode {
	case "dump":
		if err := runDump(*capturePath); err != nil {
			fmt.Fprintf(os.Stderr, "dump failed: %v\n", err)
			os.Exit(1)
		}
	case "json":
		if err := runJSON(*capturePath); err != nil {
			fmt.Fprintf(os.Stderr, "json failed: %v\n", err)
			os.Exit(1)
		}
	case "stats":
		if err := runStats(*capturePath); err != nil {
			fmt.Fprintf(os.Stderr, "stats failed: %v\n", err)
			os.Exit(1)
		}
	case "replay":
		if *target == "" {
			fmt.Fprintln(os.Stderr, "error: --target is required for replay mode")
			os.Exit(1)
		}
		if err := runReplay(*capturePath, *target, *speed); err != nil {
			fmt.Fprintf(os.Stderr, "replay failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
		os.Exit(1)
	}
}

func openCapture(path string) (*pcap.Reader, *os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open capture: %w", err)
	}
	r, err := pcap.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("read capture: %w", err)
	}
	return r, f, nil
}

func readAllPackets(r *pcap.Reader) ([]pcap.PacketRecord, error) {
	var records []pcap.PacketRecord
	for {
		rec, err := r.ReadPacket()
		if err == io.EOF {
			break
		}
		if err != nil {
			return records, err
		}
		records = append(records, rec)
	}
	return records, nil
}

func runReplay(path, target string, speed float64) error {
	r, f, err := openCapture(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	records, err := readAllPackets(r)
	if err != nil {
		return err
	}

	c2s := pcap.FilterByDirection(records, pcap.DirClientToServer)
	expectedS2C := pcap.FilterByDirection(records, pcap.DirServerToClient)

	if len(c2s) == 0 {
		fmt.Println("No C→S packets in capture, nothing to replay.")
		return nil
	}

	fmt.Printf("=== Replay: %s ===\n", path)
	fmt.Printf("Server type: %s  Target: %s  Speed: %.1fx\n", r.Header.ServerType, target, speed)
	fmt.Printf("C→S packets to send: %d  Expected S→C responses: %d\n\n", len(c2s), len(expectedS2C))

	// Connect based on server type.
	var mhf *conn.MHFConn
	switch r.Header.ServerType {
	case pcap.ServerTypeChannel:
		mhf, err = conn.DialDirect(target)
	default:
		mhf, err = conn.DialWithInit(target)
	}
	if err != nil {
		return fmt.Errorf("connect to %s: %w", target, err)
	}

	// Collect S→C responses concurrently.
	var actualS2C []pcap.PacketRecord
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			pkt, err := mhf.ReadPacket()
			if err != nil {
				return
			}

			var opcode uint16
			if len(pkt) >= 2 {
				opcode = binary.BigEndian.Uint16(pkt[:2])
			}

			// Auto-respond to ping to keep connection alive.
			if opcode == opcodeSysPing {
				pong := buildPingResponse()
				_ = mhf.SendPacket(pong)
			}

			mu.Lock()
			actualS2C = append(actualS2C, pcap.PacketRecord{
				TimestampNs: time.Now().UnixNano(),
				Direction:   pcap.DirServerToClient,
				Opcode:      opcode,
				Payload:     pkt,
			})
			mu.Unlock()
		}
	}()

	// Send C→S packets with timing.
	var lastTs int64
	for i, pkt := range c2s {
		if i > 0 && speed > 0 {
			delta := time.Duration(float64(pkt.TimestampNs-lastTs) / speed)
			if delta > 0 {
				time.Sleep(delta)
			}
		}
		lastTs = pkt.TimestampNs
		opcodeName := network.PacketID(pkt.Opcode).String()
		fmt.Printf("[replay] #%d sending 0x%04X %-30s (%d bytes)\n", i, pkt.Opcode, opcodeName, len(pkt.Payload))
		if err := mhf.SendPacket(pkt.Payload); err != nil {
			fmt.Printf("[replay] send error: %v\n", err)
			break
		}
	}

	// Wait for remaining responses.
	fmt.Println("\n[replay] All packets sent, waiting for remaining responses...")
	time.Sleep(2 * time.Second)
	_ = mhf.Close()
	<-done

	// Compare.
	mu.Lock()
	diffs := ComparePackets(expectedS2C, actualS2C)
	mu.Unlock()

	// Report.
	fmt.Printf("\n=== Replay Results ===\n")
	fmt.Printf("Sent: %d C→S packets\n", len(c2s))
	fmt.Printf("Expected: %d S→C responses\n", len(expectedS2C))
	fmt.Printf("Received: %d S→C responses\n", len(actualS2C))
	fmt.Printf("Differences: %d\n\n", len(diffs))
	for _, d := range diffs {
		fmt.Println(d.String())
	}

	if len(diffs) == 0 {
		fmt.Println("All responses match!")
	}

	return nil
}

// buildPingResponse builds a minimal MSG_SYS_PING response packet.
// Format: [opcode 0x0017][0x00 0x10 terminator]
func buildPingResponse() []byte {
	return []byte{0x00, 0x17, 0x00, 0x10}
}

func runDump(path string) error {
	r, f, err := openCapture(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Print header info.
	startTime := time.Unix(0, r.Header.SessionStartNs)
	fmt.Printf("=== MHFR Capture: %s ===\n", path)
	fmt.Printf("Server: %s  ClientMode: %d  Start: %s\n",
		r.Header.ServerType, r.Header.ClientMode, startTime.Format(time.RFC3339Nano))
	if r.Meta.Host != "" {
		fmt.Printf("Host: %s  Port: %d  Remote: %s\n", r.Meta.Host, r.Meta.Port, r.Meta.RemoteAddr)
	}
	if r.Meta.CharID != 0 {
		fmt.Printf("CharID: %d  UserID: %d\n", r.Meta.CharID, r.Meta.UserID)
	}
	fmt.Println()

	records, err := readAllPackets(r)
	if err != nil {
		return err
	}

	for i, rec := range records {
		elapsed := time.Duration(rec.TimestampNs - r.Header.SessionStartNs)
		opcodeName := network.PacketID(rec.Opcode).String()
		fmt.Printf("#%04d  +%-12s  %s  0x%04X %-30s  %d bytes\n",
			i, elapsed, rec.Direction, rec.Opcode, opcodeName, len(rec.Payload))
	}

	fmt.Printf("\nTotal: %d packets\n", len(records))
	return nil
}

type jsonCapture struct {
	Header  jsonHeader           `json:"header"`
	Meta    pcap.SessionMetadata `json:"metadata"`
	Packets []jsonPacket         `json:"packets"`
}

type jsonHeader struct {
	Version    uint16 `json:"version"`
	ServerType string `json:"server_type"`
	ClientMode int    `json:"client_mode"`
	StartTime  string `json:"start_time"`
}

type jsonPacket struct {
	Index      int    `json:"index"`
	Timestamp  string `json:"timestamp"`
	ElapsedNs  int64  `json:"elapsed_ns"`
	Direction  string `json:"direction"`
	Opcode     uint16 `json:"opcode"`
	OpcodeName string `json:"opcode_name"`
	PayloadLen int    `json:"payload_len"`
}

func runJSON(path string) error {
	r, f, err := openCapture(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	records, err := readAllPackets(r)
	if err != nil {
		return err
	}

	out := jsonCapture{
		Header: jsonHeader{
			Version:    r.Header.Version,
			ServerType: r.Header.ServerType.String(),
			ClientMode: int(r.Header.ClientMode),
			StartTime:  time.Unix(0, r.Header.SessionStartNs).Format(time.RFC3339Nano),
		},
		Meta:    r.Meta,
		Packets: make([]jsonPacket, len(records)),
	}

	for i, rec := range records {
		out.Packets[i] = jsonPacket{
			Index:      i,
			Timestamp:  time.Unix(0, rec.TimestampNs).Format(time.RFC3339Nano),
			ElapsedNs:  rec.TimestampNs - r.Header.SessionStartNs,
			Direction:  rec.Direction.String(),
			Opcode:     rec.Opcode,
			OpcodeName: network.PacketID(rec.Opcode).String(),
			PayloadLen: len(rec.Payload),
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func runStats(path string) error {
	r, f, err := openCapture(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	records, err := readAllPackets(r)
	if err != nil {
		return err
	}

	if len(records) == 0 {
		fmt.Println("Empty capture (0 packets)")
		return nil
	}

	// Compute stats.
	type opcodeStats struct {
		opcode uint16
		count  int
		bytes  int
	}
	statsMap := make(map[uint16]*opcodeStats)
	var totalC2S, totalS2C int
	var bytesC2S, bytesS2C int

	for _, rec := range records {
		s, ok := statsMap[rec.Opcode]
		if !ok {
			s = &opcodeStats{opcode: rec.Opcode}
			statsMap[rec.Opcode] = s
		}
		s.count++
		s.bytes += len(rec.Payload)

		switch rec.Direction {
		case pcap.DirClientToServer:
			totalC2S++
			bytesC2S += len(rec.Payload)
		case pcap.DirServerToClient:
			totalS2C++
			bytesS2C += len(rec.Payload)
		}
	}

	// Sort by count descending.
	sorted := make([]*opcodeStats, 0, len(statsMap))
	for _, s := range statsMap {
		sorted = append(sorted, s)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	duration := time.Duration(records[len(records)-1].TimestampNs - records[0].TimestampNs)

	fmt.Printf("=== Capture Stats: %s ===\n", path)
	fmt.Printf("Server: %s  Duration: %s  Packets: %d\n",
		r.Header.ServerType, duration, len(records))
	fmt.Printf("C→S: %d packets (%d bytes)  S→C: %d packets (%d bytes)\n\n",
		totalC2S, bytesC2S, totalS2C, bytesS2C)

	fmt.Printf("%-8s %-35s %8s %10s\n", "Opcode", "Name", "Count", "Bytes")
	fmt.Printf("%-8s %-35s %8s %10s\n", "------", "----", "-----", "-----")
	for _, s := range sorted {
		name := network.PacketID(s.opcode).String()
		fmt.Printf("0x%04X   %-35s %8d %10d\n", s.opcode, name, s.count, s.bytes)
	}

	return nil
}
