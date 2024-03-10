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

	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/memblob"
	_ "gocloud.dev/blob/s3blob"
	_ "gocloud.dev/pubsub/mempubsub"
)

func main() {
	var (
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err       error
	)

	if err = command.NewMomo().ExecuteContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err.Error())
	}

	stop()
	xos.ExitFromError(err)
}
