package daemon

import (
	"encr.dev/cli/daemon/run"
	daemonpb "encr.dev/proto/afterpiece/daemon"
)

// Check checks the app for compilation errors.
func (s *Server) Check(req *daemonpb.CheckRequest, stream daemonpb.Daemon_CheckServer) error {
	slog := &streamLog{stream: stream, buffered: false}
	log := newStreamLogger(slog)

	app, err := s.apps.Track(req.AppRoot)
	if err != nil {
		log.Error().Err(err).Msg("failed to resolve app")
		streamExit(stream, 1)
		return nil
	}

	buildDir, err := s.mgr.Check(stream.Context(), run.CheckParams{
		App:          app,
		WorkingDir:   req.WorkingDir,
		CodegenDebug: req.CodegenDebug,
		Environ:      req.Environ,
		Tests:        req.ParseTests,
	})

	exitCode := 0
	if err != nil {
		exitCode = 1
		// Send structured error data if available, so that consumers
		// like the LSP server can extract file locations and diagnostics.
		if errList := run.AsErrorList(err); errList != nil {
			slog.Error(errList)
		} else {
			log.Error().Msg(err.Error())
		}
	}

	if req.CodegenDebug && buildDir != "" {
		log.Info().Msgf("wrote generated code to: %s", buildDir)
	}
	streamExit(stream, exitCode)
	return nil
}
