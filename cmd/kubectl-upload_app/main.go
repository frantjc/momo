package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/frantjc/momo/command"
	xos "github.com/frantjc/x/os"
)

func main() {
	var (
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err       error
	)

	if err = command.SetCommon(command.NewKubectlUploadApp(), SemVer()).ExecuteContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		stop()
		xos.ExitFromError(err)
	}

	stop()
}
