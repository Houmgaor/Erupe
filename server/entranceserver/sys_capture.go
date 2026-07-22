package entranceserver

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"erupe-ce/network"
	"erupe-ce/network/pcap"

	"go.uber.org/zap"
)

// startEntranceCapture wraps a Conn with a RecordingConn if capture is enabled for entrance server.
func startEntranceCapture(s *Server, conn network.Conn, remoteAddr net.Addr) (network.Conn, func()) {
	capCfg := s.erupeConfig.Capture
	if !capCfg.Enabled || !capCfg.CaptureEntrance {
		return conn, func() {}
	}

	outputDir := capCfg.OutputDir
	if outputDir == "" {
		outputDir = "captures"
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		s.logger.Warn("Failed to create capture directory", zap.Error(err))
		return conn, func() {}
	}

	now := time.Now()
	filename := fmt.Sprintf("entrance_%s_%s.mhfr",
		now.Format("20060102_150405"),
		sanitizeAddr(remoteAddr.String()),
	)
	path := filepath.Join(outputDir, filename)

	f, err := os.Create(path)
	if err != nil {
		s.logger.Warn("Failed to create capture file", zap.Error(err), zap.String("path", path))
		return conn, func() {}
	}

	startNs := now.UnixNano()
	hdr := pcap.FileHeader{
		Version:        pcap.FormatVersion,
		ServerType:     pcap.ServerTypeEntrance,
		ClientMode:     byte(s.erupeConfig.RealClientMode),
		SessionStartNs: startNs,
	}
	meta := pcap.SessionMetadata{
		Host:       s.erupeConfig.Host,
		Port:       int(s.erupeConfig.Entrance.Port),
		RemoteAddr: remoteAddr.String(),
	}

	w, err := pcap.NewWriter(f, hdr, meta)
	if err != nil {
		s.logger.Warn("Failed to initialize capture writer", zap.Error(err))
		_ = f.Close()
		return conn, func() {}
	}

	s.logger.Info("Capture started", zap.String("file", path))

	rc := pcap.NewRecordingConn(conn, w, startNs, capCfg.ExcludeOpcodes)
	cleanup := func() {
		if err := w.Flush(); err != nil {
			s.logger.Warn("Failed to flush capture", zap.Error(err))
		}
		if err := f.Close(); err != nil {
			s.logger.Warn("Failed to close capture file", zap.Error(err))
		}
		s.logger.Info("Capture saved", zap.String("file", path))
	}

	return rc, cleanup
}

func sanitizeAddr(addr string) string {
	out := make([]byte, 0, len(addr))
	for i := 0; i < len(addr); i++ {
		c := addr[i]
		if c == ':' {
			out = append(out, '_')
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}
