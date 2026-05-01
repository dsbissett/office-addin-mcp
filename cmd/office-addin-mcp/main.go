// Command office-addin-mcp speaks the Model Context Protocol over stdio for
// driving Office add-ins running in WebView2 over CDP.
//
// Phase 1 surface: a single binary entry. Subsequent phases add manifest-aware
// launch helpers and high-level Excel/page tools.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/dsbissett/office-addin-mcp/internal/launch"
	mcpserver "github.com/dsbissett/office-addin-mcp/internal/mcp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

const (
	dangerousEnvVar    = "OAMCP_ALLOW_DANGEROUS_CDP"
	exposeRawCDPEnvVar = "OFFICE_ADDIN_MCP_EXPOSE_RAW_CDP"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("office-addin-mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { writeUsage(fs.Output()) }

	var (
		showVersion    = fs.Bool("version", false, "print binary version and exit")
		browserURL     = fs.String("browser-url", "", "Chrome DevTools HTTP endpoint (default: probe http://127.0.0.1:9222)")
		wsEndpoint     = fs.String("ws-endpoint", "", "Direct browser WebSocket endpoint (overrides --browser-url)")
		logFile        = fs.String("log-file", "", "Append diagnostic logs to this file (defaults to stderr)")
		allowDangerous = fs.Bool("allow-dangerous-cdp", false, "Allow CDP methods marked dangerous (Browser.crash, Runtime.terminateExecution, ...). May also be set via "+dangerousEnvVar+"=1.")
		exposeRawCDP   = fs.Bool("expose-raw-cdp", false, "Also register the ~411 code-generated cdp.* tools (raw Chrome DevTools Protocol). May also be set via "+exposeRawCDPEnvVar+"=1.")
	)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument: %q\n", fs.Arg(0))
		fmt.Fprintln(stderr, "office-addin-mcp now speaks MCP over stdio; the call/serve/daemon subcommands have been removed.")
		fmt.Fprintln(stderr, "Run with --help for available flags.")
		return 2
	}

	logSink := stderr
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintf(stderr, "open log file: %v\n", err)
			return 1
		}
		defer func() { _ = f.Close() }()
		logSink = f
	}

	dangerous := *allowDangerous || envFlagSet(dangerousEnvVar)
	rawCDP := *exposeRawCDP || envFlagSet(exposeRawCDPEnvVar)

	sessMgr := session.NewManager(session.Config{})
	defer sessMgr.Close()
	// Make sure any add-in launches we own are stopped on clean shutdown.
	// Signal-driven termination already runs through signal.NotifyContext
	// below; this defer covers the normal Run-returns path.
	defer launch.StopAll()

	srv := mcpserver.NewServer(mcpserver.Options{
		Name:    "office-addin-mcp",
		Version: version,
		Endpoint: webview2.Config{
			WSEndpoint: *wsEndpoint,
			BrowserURL: *browserURL,
		},
		AllowDangerous: dangerous,
		Registry:       mcpserver.DefaultRegistry(rawCDP),
		Sessions:       sessMgr,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		fmt.Fprintf(logSink, "mcp server: %v\n", err)
		return 1
	}
	return 0
}

func envFlagSet(name string) bool {
	switch os.Getenv(name) {
	case "1", "true", "TRUE", "yes":
		return true
	}
	return false
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: office-addin-mcp [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Speaks Model Context Protocol over stdio. Plug into any MCP-compatible client")
	fmt.Fprintln(w, "(Cursor, Cline, VS Code, etc.).")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "flags:")
	fmt.Fprintln(w, "  --browser-url           Chrome DevTools HTTP endpoint (default: http://127.0.0.1:9222)")
	fmt.Fprintln(w, "  --ws-endpoint           Direct browser WebSocket URL (overrides --browser-url)")
	fmt.Fprintln(w, "  --log-file              Append diagnostics here instead of stderr")
	fmt.Fprintln(w, "  --allow-dangerous-cdp   Permit dangerous CDP methods (env: "+dangerousEnvVar+")")
	fmt.Fprintln(w, "  --expose-raw-cdp        Register the raw cdp.* tool surface (env: "+exposeRawCDPEnvVar+")")
	fmt.Fprintln(w, "  --version               Print version and exit")
}
