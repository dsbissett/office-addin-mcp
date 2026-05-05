// Package mcp adapts the office-addin-mcp tool registry and dispatcher onto
// the official MCP Go SDK over a stdio transport. The dispatcher, registry,
// session manager, and JSON Schema validation continue to own all
// behavior — this package only translates between the SDK's wire types and
// our existing tools.Envelope / tools.Request shapes.
package mcp

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// Options configures the MCP stdio server.
type Options struct {
	// Name and Version populate the SDK Implementation block sent on initialize.
	Name    string
	Version string
	// Endpoint is the default CDP endpoint config used for every dispatched
	// tool call. Phase 1 sets this once at process start; later phases
	// (addin.launch) will mutate it after sideloading Excel.
	Endpoint webview2.Config
	// AllowDangerous propagates into the dispatcher and gates dangerous CDP
	// methods (Browser.crash, Runtime.terminateExecution, ...).
	AllowDangerous bool
	// Registry is the tool set to expose; required.
	Registry *tools.Registry
	// Sessions is the session.Manager used by the dispatcher. If nil a fresh
	// manager with default config is created.
	Sessions *session.Manager
	// DocCache is the persistent document discovery cache. nil falls back to
	// a default-path enabled store; pass doccache.Open("", true) to disable.
	DocCache *doccache.Store
}

// Server wraps an SDK *mcp.Server bound to the office-addin-mcp dispatcher.
type Server struct {
	sdk  *sdk.Server
	disp *tools.Dispatcher

	endpointMu sync.RWMutex
	endpoint   webview2.Config

	manifestMu sync.RWMutex
	manifest   *addin.Manifest
}

// NewServer wires the SDK server, dispatcher, and tool handlers together.
// Tool registration happens here so the SDK's tools/list response is fully
// populated by the time Run is called.
func NewServer(opts Options) *Server {
	if opts.Registry == nil {
		panic("mcp.NewServer: Registry is required")
	}
	if opts.Name == "" {
		opts.Name = "office-addin-mcp"
	}
	if opts.Version == "" {
		opts.Version = "0.0.0-dev"
	}
	if opts.Sessions == nil {
		opts.Sessions = session.NewManager(session.Config{})
	}
	if opts.DocCache == nil {
		opts.DocCache = doccache.Open("", false)
	}

	sdkServer := sdk.NewServer(&sdk.Implementation{
		Name:    opts.Name,
		Version: opts.Version,
	}, nil)

	s := &Server{sdk: sdkServer, endpoint: opts.Endpoint}
	s.disp = &tools.Dispatcher{
		Registry:       opts.Registry,
		Sessions:       opts.Sessions,
		AllowDangerous: opts.AllowDangerous,
		SetEndpoint:    s.setEndpoint,
		Manifest:       s.currentManifest,
		SetManifest:    s.setManifest,
		DocCache:       opts.DocCache,
	}
	for _, t := range opts.Registry.List() {
		s.registerTool(t)
	}
	return s
}

// setEndpoint atomically replaces the default CDP endpoint used by every
// dispatched tool call. Wired through Dispatcher.SetEndpoint so addin.launch
// can switch the server over to the freshly sideloaded Excel without the
// caller having to pass --browser-url.
func (s *Server) setEndpoint(cfg webview2.Config) {
	s.endpointMu.Lock()
	s.endpoint = cfg
	s.endpointMu.Unlock()
}

// currentEndpoint returns the active default endpoint under a read lock.
func (s *Server) currentEndpoint() webview2.Config {
	s.endpointMu.RLock()
	defer s.endpointMu.RUnlock()
	return s.endpoint
}

// setManifest stores the parsed manifest for the active add-in launch.
// Surface-based selectors and addin.* tools consult it via currentManifest.
func (s *Server) setManifest(m *addin.Manifest) {
	s.manifestMu.Lock()
	s.manifest = m
	s.manifestMu.Unlock()
}

// currentManifest returns the active manifest under a read lock.
func (s *Server) currentManifest() *addin.Manifest {
	s.manifestMu.RLock()
	defer s.manifestMu.RUnlock()
	return s.manifest
}

// Run starts the MCP stdio loop. Blocks until the peer disconnects (stdin
// closes) or ctx is canceled.
func (s *Server) Run(ctx context.Context) error {
	if err := s.sdk.Run(ctx, &sdk.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp serve: %w", err)
	}
	return nil
}

// SDKServer exposes the underlying SDK server for tests that connect via
// in-memory transports.
func (s *Server) SDKServer() *sdk.Server { return s.sdk }
