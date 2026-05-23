// Package mcp adapts the office-addin-mcp tool registry and dispatcher onto
// the official MCP Go SDK over a stdio transport. The dispatcher, registry,
// session manager, and JSON Schema validation continue to own all
// behavior — this package only translates between the SDK's wire types and
// our existing tools.Envelope / tools.Request shapes.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/launch"
	"github.com/dsbissett/office-addin-mcp/internal/recorder"
	"github.com/dsbissett/office-addin-mcp/internal/resources"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/macrotool"
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
	// Recorder is the macro recording store. nil disables macro recording.
	Recorder *recorder.Store
	// DisableAutoRecover turns off the self-healing path that, on a dead CDP
	// connection, automatically stops and relaunches the add-in this server
	// started. Recovery only ever touches launches this server tracked (never
	// an external --browser-url endpoint), so it is on by default.
	DisableAutoRecover bool
}

// Server wraps an SDK *mcp.Server bound to the office-addin-mcp dispatcher.
type Server struct {
	sdk  *sdk.Server
	disp *tools.Dispatcher

	resourceProvider *resources.Provider
	resourceWatcher  *resources.Watcher

	endpointMu sync.RWMutex
	endpoint   webview2.Config

	manifestMu sync.RWMutex
	manifest   *addin.Manifest

	// recoverMu serializes auto-recovery so concurrent dial failures don't each
	// spawn a duplicate Excel.
	recoverMu sync.Mutex
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

	s := &Server{endpoint: opts.Endpoint}

	// Create the dispatcher before building SDK server and resources.
	s.disp = &tools.Dispatcher{
		Registry:       opts.Registry,
		Sessions:       opts.Sessions,
		AllowDangerous: opts.AllowDangerous,
		SetEndpoint:    s.setEndpoint,
		Manifest:       s.currentManifest,
		SetManifest:    s.setManifest,
		DocCache:       opts.DocCache,
		Recorder:       opts.Recorder,
	}
	if !opts.DisableAutoRecover {
		s.disp.Recover = s.recoverConnection
	}

	// Create the resource provider.
	s.resourceProvider = &resources.Provider{
		Disp:     s.disp,
		Endpoint: s.currentEndpoint,
		Cache:    opts.DocCache,
	}

	// Create the resource watcher with a notify callback.
	s.resourceWatcher = resources.NewWatcher(s.resourceProvider, func(ctx context.Context, uri string) {
		_ = s.sdk.ResourceUpdated(ctx, &sdk.ResourceUpdatedNotificationParams{URI: uri})
	})

	// Build SDK server with subscription handlers.
	sdkServer := sdk.NewServer(&sdk.Implementation{
		Name:    opts.Name,
		Version: opts.Version,
	}, &sdk.ServerOptions{
		SubscribeHandler: func(ctx context.Context, req *sdk.SubscribeRequest) error {
			return s.resourceWatcher.Subscribe(ctx, req.Params.URI)
		},
		UnsubscribeHandler: func(ctx context.Context, req *sdk.UnsubscribeRequest) error {
			s.resourceWatcher.Unsubscribe(req.Params.URI)
			return nil
		},
	})

	s.sdk = sdkServer

	// Register tools.
	for _, t := range opts.Registry.List() {
		s.registerTool(t)
	}

	// Load and register macro tools if the recorder is available.
	if opts.Recorder != nil {
		if _, err := opts.Recorder.LoadAll(); err == nil {
			// For each loaded macro, create and register a replay tool.
			for _, macroName := range opts.Recorder.List() {
				macro, _ := opts.Recorder.Get(macroName)
				if macro != nil {
					// Create a replay tool that uses the dispatcher to execute recorded steps.
					macroTool := createMacroReplayTool(macro, s.disp)
					if err := opts.Registry.Register(macroTool); err == nil {
						s.registerTool(&macroTool)
					}
				}
			}
		}
	}

	// Register resources.
	registerResources(s.sdk, s.resourceProvider)

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

// recoveryProbeTimeout bounds the liveness probe used to decide whether a
// tracked launch still responds before we relaunch it.
const recoveryProbeTimeout = 1500 * time.Millisecond

// recoverConnection is the dispatcher's auto-recovery hook. It only acts on
// launches this server tracked — never an external --browser-url endpoint — so
// it cannot relaunch Excel the user attached to manually. Serialized so
// concurrent dial failures share one relaunch.
//
// Flow: if a tracked endpoint now responds (a peer already recovered, or the
// failure was transient), reset sessions and reuse it. Otherwise stop every
// tracked launch to clear stale office-addin-debugging sideload state, do a
// fresh LaunchExcel (which waits for CDP to actually respond), point the server
// at the new endpoint, reset the session pool, and return the live endpoint.
func (s *Server) recoverConnection(ctx context.Context) (webview2.Config, error) {
	s.recoverMu.Lock()
	defer s.recoverMu.Unlock()

	tracked := launch.ListLaunches()
	if len(tracked) == 0 {
		return webview2.Config{}, errors.New("no tracked launch to recover (endpoint may be external)")
	}

	for _, t := range tracked {
		if launch.ProbeCDPEndpoint(ctx, t.CDPURL, recoveryProbeTimeout).OK {
			s.disp.Sessions.DropAll()
			return webview2.Config{BrowserURL: t.CDPURL}, nil
		}
	}

	// All tracked launches are dead. Capture the project before StopAll clears
	// the registry, then relaunch it fresh.
	project := tracked[0].Project
	if project == nil {
		return webview2.Config{}, errors.New("tracked launch has no project to relaunch")
	}
	launch.StopAll()
	// Use a fresh context for the relaunch — the request context (ctx) may have
	// only seconds left after the initial dial failure, nowhere near the ~60 s
	// Excel needs to start. 90 s covers the dev-server + CDP-ready sequence.
	launchCtx, launchCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer launchCancel()
	res, err := launch.LaunchExcel(launchCtx, project, launch.LaunchOptions{})
	if err != nil {
		return webview2.Config{}, err
	}
	s.setEndpoint(webview2.Config{BrowserURL: res.CDPURL})
	s.disp.Sessions.DropAll()
	return webview2.Config{BrowserURL: res.CDPURL}, nil
}

// Run starts the MCP stdio loop. Blocks until the peer disconnects (stdin
// closes) or ctx is canceled.
func (s *Server) Run(ctx context.Context) error {
	defer s.resourceWatcher.Close()

	if err := s.sdk.Run(ctx, &sdk.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp serve: %w", err)
	}
	return nil
}

// SDKServer exposes the underlying SDK server for tests that connect via
// in-memory transports.
func (s *Server) SDKServer() *sdk.Server { return s.sdk }

// createMacroReplayTool creates a replay tool for a recorded macro using the
// dispatcher to execute each recorded step.
func createMacroReplayTool(macro *recorder.Macro, disp *tools.Dispatcher) tools.Tool {
	runner := func(ctx context.Context, toolName string, params json.RawMessage, env *tools.RunEnv) tools.Result {
		// Dispatch the recorded tool call through the normal tool pipeline.
		req := tools.Request{
			Tool:      toolName,
			Params:    params,
			SessionID: env.Diag.SessionID,
		}
		if env.Endpoint.WSEndpoint != "" || env.Endpoint.BrowserURL != "" {
			req.Endpoint = env.Endpoint
		}
		envelope := disp.Dispatch(ctx, req)
		if envelope.OK {
			return tools.OK(envelope.Data)
		}
		return tools.Result{Err: envelope.Error}
	}
	return macrotool.MakeMacroTool(macro, runner)
}
