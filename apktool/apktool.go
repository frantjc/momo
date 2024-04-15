package apktool

import (
	"context"
	"os/exec"
)

// Decode finds `apktool` on the PATH and runs Decode against it.
// See Command.Decode.
func Decode(ctx context.Context, name string, opts *DecodeOpts) error {
	return Command("apktool").Decode(ctx, name, opts)
}

// Command represents the path to an `apktool` executable.
type Command string

func (c Command) String() string {
	return string(c)
}

// DecodeOpts represent flags that can be passed to `apktool decode`.
type DecodeOpts struct {
	Force           bool
	NoResources     bool
	NoSources       bool
	OutputDirectory string
}

// Decode executes a command against `apktool` found at Command.
// It runs `apktool decode` against the .apk at name with flags
// derived from the given DecodeOpts.
func (c Command) Decode(ctx context.Context, name string, opts *DecodeOpts) error {
	args := []string{"decode"}

	if opts != nil {
		if opts.Force {
			args = append(args, "--force")
		}

		if opts.NoResources {
			args = append(args, "--no-res")
		}

		if opts.NoSources {
			args = append(args, "--no-src")
		}

		if opts.OutputDirectory != "" {
			args = append(args, "--output", opts.OutputDirectory)
		}
	}

	args = append(args, name)

	//nolint:gosec
	return exec.CommandContext(ctx, c.String(), args...).Run()
}
