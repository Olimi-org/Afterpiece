package lsp

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"encr.dev/cli/cmd/afterpiece/cmdutil"
	"encr.dev/cli/cmd/afterpiece/lsp/server"
	"encr.dev/cli/cmd/afterpiece/root"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts LSP server for incremental error tracking",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()

		daemon := cmdutil.ConnectDaemon(ctx)

		lspServer := server.NewLSPServer(daemon)

		// Start blocks on stdio until the connection closes.
		if err := lspServer.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "lsp server error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	lspCmd.AddCommand(startCmd)
	root.Cmd.AddCommand(lspCmd)
}

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "LSP (Language Server Protocol) server for incremental error tracking",
}
