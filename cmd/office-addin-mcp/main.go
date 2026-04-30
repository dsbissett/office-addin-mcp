// Command office-addin-mcp drives Office add-ins running in WebView2 over CDP.
//
// Phase 1 surface: version, help, and the call subcommand. Subsequent phases
// add serve/daemon/list-tools/status per PLAN.md §6.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/dsbissett/office-addin-mcp/internal/cli"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "--help", "-h":
		usage(stdout)
		return 0
	case "call":
		return cli.RunCall(args[1:], stdout, stderr)
	case "list-tools":
		return cli.RunListTools(args[1:], stdout, stderr)
	case "daemon":
		return cli.RunDaemon(args[1:], stdout, stderr)
	case "serve":
		return cli.RunServe(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown subcommand: %s\n\n", args[0])
		usage(stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: office-addin-mcp <subcommand>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "subcommands:")
	fmt.Fprintln(w, "  call         invoke a tool (e.g. --tool cdp.evaluate)")
	fmt.Fprintln(w, "  list-tools   print registered tools and JSON Schemas")
	fmt.Fprintln(w, "  daemon       run the persistent TCP server (auto-routed by call)")
	fmt.Fprintln(w, "  serve        --stdio: read JSON requests on stdin, write envelopes on stdout")
	fmt.Fprintln(w, "  version      print the binary version")
	fmt.Fprintln(w, "  help         print this message")
}
