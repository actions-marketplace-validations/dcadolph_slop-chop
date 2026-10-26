package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dcadolph/slop-chop/cmd/config"
	"github.com/dcadolph/slop-chop/internal/lsp"
)

// lspCmd builds the lsp subcommand, which runs slop-chop as a Language Server over stdio.
func lspCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lsp",
		Short: "Run as a Language Server over stdio.",
		Long: `lsp speaks the Language Server Protocol on stdin and stdout, so an editor can flag
and chop slop as you write. Tells become diagnostics, and the fix pass is offered as a
"Chop the slop" code action and as document formatting. A source file is read as its
comments, and each rewrite there is a quick fix of its own, with no chop or formatting that
would touch the code. --code-comments=false leaves source files alone. It uses the same
profile, presets, and voice as the other commands.`,
		Args: cobra.NoArgs,
		RunE: runLSP,
	}
	f := cmd.Flags()
	f.AddFlag(&config.FlagProfile)
	f.AddFlag(&config.FlagDialect)
	f.AddFlag(&config.FlagPreset)
	f.AddFlag(&config.FlagVoice)
	f.AddFlag(&config.FlagCodeComments)
	return cmd
}

// runLSP builds the sanitizer and serves the Language Server on stdio.
func runLSP(cmd *cobra.Command, _ []string) error {
	s, _, err := newSanitizer()
	if err != nil {
		return err
	}
	var opts []lsp.Option
	if !config.CodeComments() {
		opts = append(opts, lsp.ProseOnly())
	}
	return lsp.NewServer(s, cmd.InOrStdin(), cmd.OutOrStdout(), opts...).Run()
}
