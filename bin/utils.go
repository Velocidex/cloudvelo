package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/alecthomas/kingpin.v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

func FatalIfError(command *kingpin.CmdClause, cb func() error) {
	err := cb()
	kingpin.FatalIfError(err, command.FullCommand())
}

func install_sig_handler() (context.Context, context.CancelFunc) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-quit:
			// Ordered shutdown now.
			cancel()

		case <-ctx.Done():
			return
		}
	}()

	return ctx, cancel

}

func on_error(ctx context.Context, config_obj *config_proto.Config) {
	select {

	// It's ok we are supposed to exit.
	case <-ctx.Done():
		return

	default:
		// Log the error.
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Error("Exiting hard due to bug or KillKillKill! This should not happen!")

		os.Exit(-1)
	}
}
