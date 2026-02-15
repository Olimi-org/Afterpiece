package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"encr.dev/cli/internal/jsonrpc2"
)

// handler implements the LSP message handling logic.
type handler struct {
	conn    jsonrpc2.Conn
	checker *Checker

	// mu protects openFiles and lastDiagURIs.
	mu sync.Mutex
	// openFiles tracks currently open file URIs.
	openFiles map[string]bool
	// lastDiagURIs tracks URIs that had diagnostics in the last check run,
	// so we can clear them when the errors go away.
	lastDiagURIs map[string]bool

	// checkMu serializes check runs.
	checkMu sync.Mutex
	// cancelCheck cancels the in-progress check, if any.
	cancelCheck context.CancelFunc

	// debounce timer for didChange events.
	debounceMu    sync.Mutex
	debounceTimer *time.Timer
}

func newHandler(checker *Checker) *handler {
	return &handler{
		checker:      checker,
		openFiles:    make(map[string]bool),
		lastDiagURIs: make(map[string]bool),
	}
}

// setConn sets the JSON-RPC connection used for sending notifications.
func (h *handler) setConn(conn jsonrpc2.Conn) {
	h.conn = conn
}

// Handle is the jsonrpc2.Handler implementation.
func (h *handler) Handle(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	method := req.Method()

	switch method {
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
		// For unknown methods, reply with method not found for calls,
		// and silently ignore notifications.
		return jsonrpc2.MethodNotFound(ctx, reply, req)
	}
}

func (h *handler) handleInitialize(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
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
			Name:    "encore-lsp",
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

	log.Debug().Str("uri", params.TextDocument.URI).Msg("lsp: didOpen")

	// Trigger a check.
	go h.runCheck(ctx)

	return reply(ctx, nil, nil)
}

func (h *handler) handleDidChange(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	// Debounce didChange — wait 300ms before triggering a check.
	h.debounceMu.Lock()
	if h.debounceTimer != nil {
		h.debounceTimer.Stop()
	}
	h.debounceTimer = time.AfterFunc(300*time.Millisecond, func() {
		h.runCheck(context.Background())
	})
	h.debounceMu.Unlock()

	return reply(ctx, nil, nil)
}

func (h *handler) handleDidSave(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params DidSaveTextDocumentParams
	if err := unmarshalParams(req, &params); err != nil {
		return reply(ctx, nil, err)
	}

	log.Debug().Str("uri", params.TextDocument.URI).Msg("lsp: didSave")

	// Trigger a check immediately on save (cancel any debounced change check).
	h.debounceMu.Lock()
	if h.debounceTimer != nil {
		h.debounceTimer.Stop()
		h.debounceTimer = nil
	}
	h.debounceMu.Unlock()

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

	log.Debug().Str("uri", params.TextDocument.URI).Msg("lsp: didClose")

	// Clear diagnostics for the closed file.
	h.publishDiagnostics(ctx, params.TextDocument.URI, nil)

	return reply(ctx, nil, nil)
}

// runCheck triggers a full project check and publishes diagnostics.
func (h *handler) runCheck(ctx context.Context) {
	// Cancel any in-progress check.
	h.checkMu.Lock()
	if h.cancelCheck != nil {
		h.cancelCheck()
	}
	checkCtx, cancel := context.WithCancel(ctx)
	h.cancelCheck = cancel
	h.checkMu.Unlock()

	defer cancel()

	log.Debug().Msg("lsp: running check...")

	result, err := h.checker.Run(checkCtx)
	if err != nil {
		if checkCtx.Err() != nil {
			// Check was cancelled, don't log.
			return
		}
		log.Warn().Err(err).Msg("lsp: check failed")
		return
	}

	// Collect the set of URIs that have diagnostics in this run.
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

	log.Debug().Int("files_with_errors", len(result.Diagnostics)).Msg("lsp: check complete")
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
		log.Warn().Err(err).Str("uri", uri).Msg("lsp: failed to publish diagnostics")
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
