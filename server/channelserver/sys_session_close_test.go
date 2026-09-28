package channelserver

import (
	"net"
	"testing"
	"time"
)

// TestSession_EndsWhenClientDisconnects checks that a session whose client
// drops the connection without logging out ends, so sendLoop returns instead
// of outliving the connection.
func TestSession_EndsWhenClientDisconnects(t *testing.T) {
	server := createMockServer()
	serverSide, clientSide := net.Pipe()
	s := NewSession(server, serverSide)
	server.Lock()
	server.sessions[serverSide] = s
	server.Unlock()
	s.Start()

	_ = clientSide.Close()

	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatal("session did not end after the client disconnected")
	}
	if !s.closed.Load() {
		t.Error("closed not set after disconnect")
	}
}

// TestSession_MarkClosedIsIdempotent checks that ending a session twice,
// e.g. logout followed by the connection dropping, doesn't panic.
func TestSession_MarkClosedIsIdempotent(t *testing.T) {
	s := &Session{done: make(chan struct{})}
	s.markClosed()
	s.markClosed()
	select {
	case <-s.done:
	default:
		t.Error("done not closed")
	}
}
