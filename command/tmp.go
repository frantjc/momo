package command

import (
	"fmt"
	"runtime"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momoregexp"
	"github.com/spf13/cobra"
)

// NewTmp returns the root command for
// tmp which acts as its CLI entrypoint.
func NewTmp() *cobra.Command {
	var (
		verbosity int
		cmd       = &cobra.Command{
			Use:           "tmp",
			Version:       momo.SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
			PreRun: func(cmd *cobra.Command, _ []string) {
				cmd.SetContext(
					momo.WithLogger(
						cmd.Context(), momo.NewLogger().V(2-verbosity),
					),
				)
			},
			RunE: func(cmd *cobra.Command, args []string) error {
				var (
					ctx = cmd.Context()
					log = momo.LoggerFrom(ctx)
					_   = log
				)

				fmt.Println(momoregexp.IsAppName(args[0]))

				return nil
			},
		}
	)

	cmd.SetVersionTemplate("{{ .Name }}{{ .Version }} " + runtime.Version() + "\n")
	cmd.Flags().CountVarP(&verbosity, "verbose", "V", "verbosity for momo")

	return cmd
}
