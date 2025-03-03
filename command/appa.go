package command

import (
	"github.com/spf13/cobra"
)

// NewAppa returns the command which acts as
// the entrypoint for `appa`.
func NewAppa() *cobra.Command {
	var (
		cmd = &cobra.Command{Use: "appa"}
	)

	cmd.AddCommand(
		NewUpload(),
	)

	return cmd
}

// NewUpload returns the command which acts as
// the entrypoint for `appa upload`.
func NewUpload() *cobra.Command {
	var (
		cmd = &cobra.Command{Use: "upload"}
	)

	cmd.AddCommand(
		NewUploadApp(),
	)

	return cmd
}
