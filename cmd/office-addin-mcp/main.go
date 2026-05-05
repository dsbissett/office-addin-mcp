// Command office-addin-mcp speaks the Model Context Protocol over stdio for
// driving Office add-ins running in WebView2 over CDP.
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

	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/launch"
	mcpserver "github.com/dsbissett/office-addin-mcp/internal/mcp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

const dangerousEnvVar = "OAMCP_ALLOW_DANGEROUS_CDP"

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
		launchAddin    = fs.Bool("launch-addin", false, "On startup, if no CDP endpoint is reachable, detect the add-in project under cwd and run addin.launch automatically. Works for any Office host (Excel, Word, Outlook, PowerPoint, OneNote). Equivalent to calling addin.ensureRunning at boot.")
		launchExcel    = fs.Bool("launch-excel", false, "Deprecated alias for --launch-addin. Kept for backwards compatibility.")
		allowDangerous = fs.Bool("allow-dangerous-cdp", false, "Allow CDP methods marked dangerous (Browser.crash, Runtime.terminateExecution, ...). May also be set via "+dangerousEnvVar+"=1.")
		noDocCache     = fs.Bool("no-doccache", false, "Disable the persistent document discovery cache used by *.discover tools. Cache misses still run; nothing reads or writes "+doccache.DefaultPath()+".")
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

	sessMgr := session.NewManager(session.Config{})
	defer sessMgr.Close()
	defer launch.StopAll()

	endpoint := webview2.Config{
		WSEndpoint: *wsEndpoint,
		BrowserURL: *browserURL,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if (*launchAddin || *launchExcel) && endpoint.WSEndpoint == "" && endpoint.BrowserURL == "" {
		if launched, err := autoLaunchAddin(ctx); err != nil {
			slog.Warn("--launch-addin could not bring up the Office host", "error", err)
		} else if launched != "" {
			endpoint.BrowserURL = launched
			slog.Info("--launch-addin: CDP endpoint ready", "browser_url", launched)
		}
	}

	srv := mcpserver.NewServer(mcpserver.Options{
		Name:           "office-addin-mcp",
		Version:        version,
		Endpoint:       endpoint,
		AllowDangerous: dangerous,
		Registry:       mcpserver.DefaultRegistry(),
		Sessions:       sessMgr,
		DocCache:       doccache.Open("", *noDocCache),
	})

	if err := srv.Run(ctx); err != nil {
		slog.Error("mcp server exited with error", "error", err)
		return 1
	}
	return 0
}

func autoLaunchAddin(ctx context.Context) (string, error) {
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
	fmt.Fprintln(w, "  --launch-addin          Auto-detect+launch the Office add-in under cwd at startup if no CDP endpoint is reachable")
	fmt.Fprintln(w, "  --launch-excel          Deprecated alias for --launch-addin")
	fmt.Fprintln(w, "  --allow-dangerous-cdp   Permit dangerous CDP methods (env: "+dangerousEnvVar+")")
	fmt.Fprintln(w, "  --no-doccache           Disable the persistent document discovery cache (*.discover tools)")
	fmt.Fprintln(w, "  --version               Print version and exit")
}
