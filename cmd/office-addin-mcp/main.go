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
	"log/slog"
	"os"
	"os/signal"
	"strings"

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
		logLevel       = fs.String("log-level", "info", "Minimum slog level: debug, info, warn, error")
		launchExcel    = fs.Bool("launch-excel", false, "On startup, if no CDP endpoint is reachable, detect the add-in project under cwd and run addin.launch automatically. Equivalent to calling addin.ensureRunning at boot.")
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

	level, err := parseLogLevel(*logLevel)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(logSink, &slog.HandlerOptions{Level: level})))

	dangerous := *allowDangerous || envFlagSet(dangerousEnvVar)
	rawCDP := *exposeRawCDP || envFlagSet(exposeRawCDPEnvVar)

	sessMgr := session.NewManager(session.Config{})
	defer sessMgr.Close()
	// Make sure any add-in launches we own are stopped on clean shutdown.
	// Signal-driven termination already runs through signal.NotifyContext
	// below; this defer covers the normal Run-returns path.
	defer launch.StopAll()

	endpoint := webview2.Config{
		WSEndpoint: *wsEndpoint,
		BrowserURL: *browserURL,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// --launch-excel: probe the configured port and, if nothing's listening,
	// detect+launch from cwd before the MCP server starts. Skipped silently
	// when the user already pinned an explicit endpoint — they presumably
	// know what they're doing.
	if *launchExcel && endpoint.WSEndpoint == "" && endpoint.BrowserURL == "" {
		if launched, err := autoLaunchExcel(ctx); err != nil {
			slog.Warn("--launch-excel could not bring up Excel", "error", err)
		} else if launched != "" {
			endpoint.BrowserURL = launched
			slog.Info("--launch-excel: CDP endpoint ready", "browser_url", launched)
		}
	}

	srv := mcpserver.NewServer(mcpserver.Options{
		Name:           "office-addin-mcp",
		Version:        version,
		Endpoint:       endpoint,
		AllowDangerous: dangerous,
		Registry:       mcpserver.DefaultRegistry(rawCDP),
		Sessions:       sessMgr,
	})

	if err := srv.Run(ctx); err != nil {
		slog.Error("mcp server exited with error", "error", err)
		return 1
	}
	return 0
}

// autoLaunchExcel implements the --launch-excel startup hook. Returns the
// resolved CDP browser URL on success, "" if no add-in could be found
// (caller treats that as a soft warning — the server still starts).
func autoLaunchExcel(ctx context.Context) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getcwd: %w", err)
	}
	project, err := launch.DetectAddin(cwd)
	if err != nil {
		return "", fmt.Errorf("detect under %s: %w", cwd, err)
	}
	res, _, err := launch.LaunchIfNeeded(ctx, project, launch.LaunchOptions{})
	if err != nil {
		return "", err
	}
	return res.CDPURL, nil
}

func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("invalid --log-level %q (want debug|info|warn|error)", s)
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
	fmt.Fprintln(w, "  --log-level             slog level: debug|info|warn|error (default info)")
	fmt.Fprintln(w, "  --launch-excel          Auto-detect+launch the add-in under cwd at startup if no CDP endpoint is reachable")
	fmt.Fprintln(w, "  --allow-dangerous-cdp   Permit dangerous CDP methods (env: "+dangerousEnvVar+")")
	fmt.Fprintln(w, "  --expose-raw-cdp        Register the raw cdp.* tool surface (env: "+exposeRawCDPEnvVar+")")
	fmt.Fprintln(w, "  --version               Print version and exit")
}
