package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/daemon"
	"github.com/dsbissett/office-addin-mcp/internal/session"
)

// RunDaemon starts the long-lived daemon and blocks until SIGINT/SIGTERM.
func RunDaemon(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", 45931, "TCP port to bind on 127.0.0.1; 0 picks an ephemeral port")
	idleTimeout := fs.Duration("idle-timeout", 30*time.Minute, "GC sessions idle longer than this")
	socketPath := fs.String("socket-file", "", "override the well-known daemon socket file path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	reg := DefaultRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := daemon.Start(ctx, reg, daemon.Config{
		Port:        *port,
		IdleTimeout: *idleTimeout,
		SocketPath:  *socketPath,
		SessionCfg:  session.Config{IdleTimeout: *idleTimeout},
		Logger:      stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "daemon: start: %v\n", err)
		return 1
	}

	// Print actual address to stdout for tooling that wants to scrape it.
	fmt.Fprintf(stdout, "%s\n", srv.Addr().String())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := srv.Stop(stopCtx); err != nil {
		fmt.Fprintf(stderr, "daemon: stop: %v\n", err)
		return 1
	}
	return 0
}
