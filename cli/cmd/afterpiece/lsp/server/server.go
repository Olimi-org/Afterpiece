package server

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/rs/zerolog/log"

	"encr.dev/cli/internal/jsonrpc2"
	daemonpb "encr.dev/proto/afterpiece/daemon"
)

// LSPServer is an LSP server that runs Encore's check pipeline on file changes
// and publishes diagnostics back to the editor.
type LSPServer struct {
	daemon  daemonpb.DaemonClient
	appRoot string
	conn    jsonrpc2.Conn
}

// NewLSPServer creates a new LSP server that communicates with the given daemon.
func NewLSPServer(daemon daemonpb.DaemonClient, appRoot string) *LSPServer {
	return &LSPServer{
		daemon:  daemon,
		appRoot: appRoot,
	}
}

// Start runs the LSP server, reading from stdin and writing to stdout.
// It blocks until the context is cancelled or the connection is closed.
func (s *LSPServer) Start(ctx context.Context) error {
	log.Info().Str("app_root", s.appRoot).Msg("lsp: starting server")

	// Create a stdio connection using LSP header framing.
	// We wrap stdin/stdout in a net.Conn-compatible type for the jsonrpc2 stream.
	rwc := &stdioConn{
		reader: os.Stdin,
		writer: os.Stdout,
	}

	stream := jsonrpc2.NewHeaderStream(rwc)
	conn := jsonrpc2.NewConn(stream)
	s.conn = conn

	checker := NewChecker(s.daemon, s.appRoot)
	h := newHandler(checker)
	h.setConn(conn)

	// Start the connection handler.
	conn.Go(ctx, jsonrpc2.AsyncHandler(jsonrpc2.MustReplyHandler(h.Handle)))

	// Block until the connection is done.
	<-conn.Done()

	if err := conn.Err(); err != nil {
		log.Debug().Err(err).Msg("lsp: connection closed")
	}

	return conn.Err()
}

// Close closes the LSP server connection.
func (s *LSPServer) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// stdioConn wraps stdin/stdout as a net.Conn for the jsonrpc2 stream.
// The jsonrpc2.NewHeaderStream expects a net.Conn, but for stdio we only
// need Read and Write. We implement the interface with stubs for the
// methods we don't need.
type stdioConn struct {
	reader *os.File
	writer *os.File
}

func (c *stdioConn) Read(b []byte) (int, error)  { return c.reader.Read(b) }
func (c *stdioConn) Write(b []byte) (int, error) { return c.writer.Write(b) }
func (c *stdioConn) Close() error {
	// Don't actually close stdin/stdout.
	return nil
}

// Stub methods to satisfy net.Conn interface.
func (c *stdioConn) LocalAddr() net.Addr                { return stdioAddr{} }
func (c *stdioConn) RemoteAddr() net.Addr               { return stdioAddr{} }
func (c *stdioConn) SetDeadline(_ time.Time) error      { return nil }
func (c *stdioConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *stdioConn) SetWriteDeadline(_ time.Time) error { return nil }

type stdioAddr struct{}

func (stdioAddr) Network() string { return "stdio" }
func (stdioAddr) String() string  { return "stdio" }
