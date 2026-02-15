package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	daemonpb "encr.dev/proto/afterpiece/daemon"
)

// Checker wraps the daemon's Check gRPC call and converts errors into LSP diagnostics.
type Checker struct {
	daemon  daemonpb.DaemonClient
	appRoot string
}

// NewChecker creates a new Checker targeting the given app root.
func NewChecker(daemon daemonpb.DaemonClient, appRoot string) *Checker {
	return &Checker{
		daemon:  daemon,
		appRoot: appRoot,
	}
}

// CheckResult holds the diagnostics grouped by file URI.
type CheckResult struct {
	// Diagnostics maps file URI → diagnostics for that file.
	Diagnostics map[string][]Diagnostic
}

// Run performs a full project check via the daemon and returns diagnostics.
func (c *Checker) Run(ctx context.Context) (*CheckResult, error) {
	wd, err := os.Getwd()
	if err != nil {
		wd = ""
	}

	// Compute working dir relative to app root.
	relWd := ""
	if wd != "" {
		if rel, err := filepath.Rel(c.appRoot, wd); err == nil {
			relWd = rel
		}
	}

	stream, err := c.daemon.Check(ctx, &daemonpb.CheckRequest{
		AppRoot:    c.appRoot,
		WorkingDir: relWd,
		Environ:    os.Environ(),
	})
	if err != nil {
		return nil, fmt.Errorf("daemon check: %w", err)
	}

	result := &CheckResult{
		Diagnostics: make(map[string][]Diagnostic),
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			// Stream closed — we're done.
			break
		}

		switch m := msg.Msg.(type) {
		case *daemonpb.CommandMessage_Errors:
			if m.Errors == nil || len(m.Errors.Errinsrc) == 0 {
				// No errors (clear state).
				continue
			}

			// Deserialize the errinsrc JSON into a generic structure.
			// We can't import errinsrc.ErrInSrc directly due to internal package
			// restrictions, so we unmarshal into a compatible anonymous struct.
			var errList struct {
				List []errInSrc `json:"list"`
			}
			if err := json.Unmarshal(m.Errors.Errinsrc, &errList); err != nil {
				log.Warn().Err(err).Msg("lsp: failed to unmarshal errinsrc")
				continue
			}

			for i := range errList.List {
				c.addDiagnostics(result, &errList.List[i])
			}

		case *daemonpb.CommandMessage_Output:
			// Log output for debugging.
			if len(m.Output.Stderr) > 0 {
				log.Debug().Str("stderr", string(m.Output.Stderr)).Msg("lsp: check output")
			}

		case *daemonpb.CommandMessage_Exit:
			// Exit message — stop reading.
		}
	}

	return result, nil
}

// errInSrc is a JSON-compatible mirror of errinsrc.ErrInSrc.
// We define our own type here because the real type's fields come from
// an internal package that we cannot import from this location.
type errInSrc struct {
	Params errParams `json:"params"`
}

type errParams struct {
	Title     string        `json:"title"`
	Summary   string        `json:"summary"`
	Locations []srcLocation `json:"locations"`
}

type srcLocation struct {
	Type  uint8    `json:"type"` // 0=Error, 1=Warning, 2=Help
	Text  string   `json:"text,omitempty"`
	File  *srcFile `json:"file,omitempty"`
	Start srcPos   `json:"start"`
	End   srcPos   `json:"end"`
}

type srcFile struct {
	RelPath  string `json:"relPath,omitempty"`
	FullPath string `json:"fullPath,omitempty"`
}

type srcPos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

const (
	locTypeError   uint8 = 0
	locTypeWarning uint8 = 1
	locTypeHelp    uint8 = 2
)

// addDiagnostics converts an errInSrc into LSP diagnostics grouped by file URI.
func (c *Checker) addDiagnostics(result *CheckResult, e *errInSrc) {
	if e == nil {
		return
	}

	message := e.Params.Title
	if e.Params.Summary != "" {
		message = e.Params.Summary
	}

	if len(e.Params.Locations) == 0 {
		// No location info — can't place a diagnostic.
		return
	}

	for _, loc := range e.Params.Locations {
		if loc.File == nil {
			continue
		}

		uri := filePathToURI(loc.File.FullPath)
		if uri == "" {
			// Try relative path.
			if loc.File.RelPath != "" {
				fullPath := filepath.Join(c.appRoot, loc.File.RelPath)
				uri = filePathToURI(fullPath)
			}
		}
		if uri == "" {
			continue
		}

		severity := SeverityError
		switch loc.Type {
		case locTypeWarning:
			severity = SeverityWarning
		case locTypeHelp:
			severity = SeverityHint
		}

		text := message
		if loc.Text != "" {
			text = loc.Text
		}

		diag := Diagnostic{
			Range: Range{
				Start: Position{
					Line:      max(0, loc.Start.Line-1), // LSP is 0-based, errinsrc is 1-based
					Character: max(0, loc.Start.Col-1),
				},
				End: Position{
					Line:      max(0, loc.End.Line-1),
					Character: max(0, loc.End.Col-1),
				},
			},
			Severity: severity,
			Source:   "encore",
			Message:  text,
		}

		result.Diagnostics[uri] = append(result.Diagnostics[uri], diag)
	}
}

// filePathToURI converts an absolute file path to a file:// URI.
func filePathToURI(path string) string {
	if path == "" {
		return ""
	}
	// Ensure the path is absolute.
	if !filepath.IsAbs(path) {
		return ""
	}
	// On Unix, file:///absolute/path. The path is already /-separated.
	u := &url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}
	return u.String()
}

// uriToFilePath converts a file:// URI back to an absolute file path.
func uriToFilePath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	return filepath.FromSlash(u.Path)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
