package main

import (
	"context"
	"errors"
	"fmt"
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

	if err = command.NewTmp().ExecuteContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err.Error())
	}

	stop()
	xos.ExitFromError(err)
}
