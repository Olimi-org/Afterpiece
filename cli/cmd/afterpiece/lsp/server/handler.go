package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	"encr.dev/cli/cmd/afterpiece/cmdutil"
	"encr.dev/cli/internal/jsonrpc2"
	daemonpb "encr.dev/proto/afterpiece/daemon"
)

// handler implements the LSP message handling logic.
type handler struct {
	conn   jsonrpc2.Conn
	daemon daemonpb.DaemonClient

	// mu protects checker, openFiles, and lastDiagURIs.
	mu sync.Mutex
	// checker is lazily created after the initialize handshake resolves
	// the app root from the editor's rootUri.
	checker *Checker
	// openFiles tracks currently open file URIs.
	openFiles map[string]bool
	// lastDiagURIs tracks URIs that had diagnostics in the last check run,
	// so we can clear them when the errors go away.
	lastDiagURIs map[string]bool

	// cancelCheck cancels the in-progress check, if any.
	// Protected by mu.
	cancelCheck context.CancelFunc
}

func newHandler(daemon daemonpb.DaemonClient) *handler {
	return &handler{
		daemon:       daemon,
		openFiles:    make(map[string]bool),
		lastDiagURIs: make(map[string]bool),
	}
}

// setConn sets the JSON-RPC connection used for sending notifications.
func (h *handler) setConn(conn jsonrpc2.Conn) {
	h.conn = conn
	logConn = conn
}

// logConn is set once the JSON-RPC connection is established, allowing
// lspLog() to send window/logMessage notifications to the editor.
var logConn jsonrpc2.Conn

// lspLog sends a message to the editor via LSP window/logMessage.
func lspLog(format string, args ...any) {
	if c := logConn; c != nil {
		_ = c.Notify(context.Background(), "window/logMessage", LogMessageParams{
			Type:    MessageLog,
			Message: fmt.Sprintf(format, args...),
		})
	}
}

// Handle is the jsonrpc2.Handler implementation.
func (h *handler) Handle(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	switch req.Method() {
	case "initialize":
		return h.handleInitialize(ctx, reply, req)
	case "initialized":
		return reply(ctx, nil, nil)
	case "shutdown":
		return reply(ctx, nil, nil)
	case "exit":
		return reply(ctx, nil, nil)
	case "textDocument/didOpen":
		return h.handleDidOpen(ctx, reply, req)
	case "textDocument/didChange":
		return h.handleDidChange(ctx, reply, req)
	case "textDocument/didSave":
		return h.handleDidSave(ctx, reply, req)
	case "textDocument/didClose":
		return h.handleDidClose(ctx, reply, req)
	default:
		return jsonrpc2.MethodNotFound(ctx, reply, req)
	}
}

func (h *handler) handleInitialize(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params InitializeParams
	if err := unmarshalParams(req, &params); err != nil {
		return reply(ctx, nil, err)
	}

	rootDir := uriToFilePath(params.RootURI)
	if rootDir == "" {
		return reply(ctx, nil, fmt.Errorf("initialize: missing rootUri"))
	}

	// Try upward search first (handles case where editor opens a subdirectory).
	appRoot, _, err := cmdutil.FindAppRootFromDir(rootDir)
	if err != nil {
		// Fall back to downward search for monorepo layouts where
		// encore.app is in a subdirectory of the workspace root.
		appRoot = findAppRootDown(rootDir, 5)
	}

	h.mu.Lock()
	if appRoot != "" {
		h.checker = NewChecker(h.daemon, appRoot)
		lspLog("initialized with app root: %s", appRoot)
	} else {
		lspLog("no encore.app found from rootUri: %s", params.RootURI)
	}
	h.mu.Unlock()

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: &TextDocumentSyncOptions{
				OpenClose: true,
				Change:    SyncFull,
				Save: &SaveOptions{
					IncludeText: false,
				},
			},
		},
		ServerInfo: &ServerInfo{
			Name:    "afterpiece-lsp",
			Version: "0.1.0",
		},
	}
	return reply(ctx, result, nil)
}

func (h *handler) handleDidOpen(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params DidOpenTextDocumentParams
	if err := unmarshalParams(req, &params); err != nil {
		return reply(ctx, nil, err)
	}

	h.mu.Lock()
	h.openFiles[params.TextDocument.URI] = true
	h.mu.Unlock()

	go h.runCheck(ctx)
	return reply(ctx, nil, nil)
}

func (h *handler) handleDidChange(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params DidChangeTextDocumentParams
	if err := unmarshalParams(req, &params); err != nil {
		return reply(ctx, nil, err)
	}

	// The daemon checks files on disk, not the in-memory buffer, so
	// re-running the checker here would just re-check the saved version.
	// Instead, clear any existing diagnostics for this file so that
	// stale squiggles at potentially wrong positions don't mislead the user.
	// Fresh diagnostics will arrive on the next save.
	uri := params.TextDocument.URI
	h.mu.Lock()
	hadDiags := h.lastDiagURIs[uri]
	h.mu.Unlock()

	if hadDiags {
		h.publishDiagnostics(ctx, uri, nil)
	}

	return reply(ctx, nil, nil)
}

func (h *handler) handleDidSave(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	go h.runCheck(ctx)
	return reply(ctx, nil, nil)
}

func (h *handler) handleDidClose(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params DidCloseTextDocumentParams
	if err := unmarshalParams(req, &params); err != nil {
		return reply(ctx, nil, err)
	}

	h.mu.Lock()
	delete(h.openFiles, params.TextDocument.URI)
	h.mu.Unlock()

	h.publishDiagnostics(ctx, params.TextDocument.URI, nil)
	return reply(ctx, nil, nil)
}

// runCheck triggers a full project check and publishes diagnostics.
func (h *handler) runCheck(ctx context.Context) {
	h.mu.Lock()
	checker := h.checker
	// Cancel any in-progress check so we don't pile up.
	if h.cancelCheck != nil {
		h.cancelCheck()
	}
	checkCtx, cancel := context.WithCancel(ctx)
	h.cancelCheck = cancel
	h.mu.Unlock()

	defer cancel()

	if checker == nil {
		return
	}

	result, err := checker.Run(checkCtx)
	if err != nil {
		if checkCtx.Err() == nil {
			lspLog("check failed: %v", err)
		}
		return
	}

	// Publish diagnostics for files with errors.
	currentURIs := make(map[string]bool)
	for uri, diags := range result.Diagnostics {
		currentURIs[uri] = true
		h.publishDiagnostics(checkCtx, uri, diags)
	}

	// Clear diagnostics for URIs that had errors before but don't anymore.
	h.mu.Lock()
	previousURIs := h.lastDiagURIs
	h.lastDiagURIs = currentURIs
	h.mu.Unlock()

	for uri := range previousURIs {
		if !currentURIs[uri] {
			h.publishDiagnostics(checkCtx, uri, nil)
		}
	}
}

// publishDiagnostics sends a textDocument/publishDiagnostics notification.
func (h *handler) publishDiagnostics(ctx context.Context, uri string, diags []Diagnostic) {
	if h.conn == nil {
		return
	}

	if diags == nil {
		diags = []Diagnostic{}
	}

	params := PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	}

	if err := h.conn.Notify(ctx, "textDocument/publishDiagnostics", params); err != nil {
		lspLog("failed to publish diagnostics: %v", err)
	}
}

// unmarshalParams extracts the params from a JSON-RPC request.
func unmarshalParams(req jsonrpc2.Request, v interface{}) error {
	params := req.Params()
	if len(params) == 0 {
		return nil
	}
	return json.Unmarshal(params, v)
}

// findAppRootDown searches downward from dir for an encore.app file,
// up to maxDepth levels deep. Returns the directory containing encore.app,
// or "" if not found.
func findAppRootDown(dir string, maxDepth int) string {
	var found string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}

		// Check depth limit.
		rel, _ := filepath.Rel(dir, path)
		depth := len(filepath.SplitList(rel))
		if depth > maxDepth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if !d.IsDir() && d.Name() == "encore.app" {
			found = filepath.Dir(path)
			return fs.SkipAll // stop walking
		}

		// Skip hidden and vendor directories.
		if d.IsDir() && (d.Name()[0] == '.' || d.Name() == "vendor" || d.Name() == "node_modules") {
			return fs.SkipDir
		}

		return nil
	})
	return found
}
