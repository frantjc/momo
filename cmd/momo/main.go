package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/928799934/go-png-cgbi"
	"github.com/frantjc/momo/command"
	xos "github.com/frantjc/x/os"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/s3blob"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	var (
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err       error
	)

	if err = command.SetCommon(command.NewMomo(), SemVer()).ExecuteContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		os.Stderr.WriteString(err.Error() + "\n")
		stop()
		xos.ExitFromError(err)
	}

	stop()
}
