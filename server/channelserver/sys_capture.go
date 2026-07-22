package channelserver

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

// startCapture wraps a network.Conn with a RecordingConn if capture is enabled.
// Returns the (possibly wrapped) conn, the RecordingConn (nil if capture disabled),
// and a cleanup function that must be called on session close.
func startCapture(server *Server, conn network.Conn, remoteAddr net.Addr, serverType pcap.ServerType) (network.Conn, *pcap.RecordingConn, func()) {
	capCfg := server.erupeConfig.Capture
	if !capCfg.Enabled {
		return conn, nil, func() {}
	}

	switch serverType {
	case pcap.ServerTypeSign:
		if !capCfg.CaptureSign {
			return conn, nil, func() {}
		}
	case pcap.ServerTypeEntrance:
		if !capCfg.CaptureEntrance {
			return conn, nil, func() {}
		}
	case pcap.ServerTypeChannel:
		if !capCfg.CaptureChannel {
			return conn, nil, func() {}
		}
	}

	outputDir := capCfg.OutputDir
	if outputDir == "" {
		outputDir = "captures"
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		server.logger.Warn("Failed to create capture directory", zap.Error(err))
		return conn, nil, func() {}
	}

	now := time.Now()
	filename := fmt.Sprintf("%s_%s_%s.mhfr",
		serverType.String(),
		now.Format("20060102_150405"),
		sanitizeAddr(remoteAddr.String()),
	)
	path := filepath.Join(outputDir, filename)

	f, err := os.Create(path)
	if err != nil {
		server.logger.Warn("Failed to create capture file", zap.Error(err), zap.String("path", path))
		return conn, nil, func() {}
	}

	startNs := now.UnixNano()
	hdr := pcap.FileHeader{
		Version:        pcap.FormatVersion,
		ServerType:     serverType,
		ClientMode:     byte(server.erupeConfig.RealClientMode),
		SessionStartNs: startNs,
	}
	meta := pcap.SessionMetadata{
		Host:       server.erupeConfig.Host,
		RemoteAddr: remoteAddr.String(),
	}

	w, err := pcap.NewWriter(f, hdr, meta)
	if err != nil {
		server.logger.Warn("Failed to initialize capture writer", zap.Error(err))
		_ = f.Close()
		return conn, nil, func() {}
	}

	server.logger.Info("Capture started", zap.String("file", path))

	rc := pcap.NewRecordingConn(conn, w, startNs, capCfg.ExcludeOpcodes)
	rc.SetCaptureFile(f, &meta)
	cleanup := func() {
		if err := w.Flush(); err != nil {
			server.logger.Warn("Failed to flush capture", zap.Error(err))
		}
		if err := f.Close(); err != nil {
			server.logger.Warn("Failed to close capture file", zap.Error(err))
		}
		server.logger.Info("Capture saved", zap.String("file", path))
	}

	return rc, rc, cleanup
}

// sanitizeAddr replaces characters that are problematic in filenames.
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
