package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// Config controls daemon startup.
type Config struct {
	Host        string         // bind address; defaults to 127.0.0.1
	Port        int            // listen port; 0 picks an ephemeral port
	IdleTimeout time.Duration  // session idle GC interval; 0 disables
	SessionCfg  session.Config // overrides reconnect budget defaults
	SocketPath  string         // override well-known path; empty uses SocketFilePath()
	Logger      io.Writer      // log destination; nil = os.Stderr
}

// Server is a running daemon. Always created via Start so the listener and
// session manager are wired before the HTTP handler runs.
type Server struct {
	cfg      Config
	listener net.Listener
	sessions *session.Manager
	registry *tools.Registry
	server   *http.Server
	token    string
	socket   string
	logger   io.Writer

	startMu sync.Mutex
	started bool
	stopped bool
}

// Start binds the listener, writes the socket file, and spawns the HTTP
// serve goroutine. Returns once the listener is accepting; cancellation of
// ctx during binding aborts startup.
func Start(ctx context.Context, registry *tools.Registry, cfg Config) (*Server, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Logger == nil {
		cfg.Logger = os.Stderr
	}
	if cfg.SocketPath == "" {
		p, err := SocketFilePath()
		if err != nil {
			return nil, err
		}
		cfg.SocketPath = p
	}
	scfg := cfg.SessionCfg
	if scfg.IdleTimeout == 0 {
		scfg.IdleTimeout = cfg.IdleTimeout
	}
	if scfg.IdleTimeout == 0 {
		scfg.IdleTimeout = 30 * time.Minute
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("daemon: listen %s: %w", addr, err)
	}

	tcpAddr, _ := ln.Addr().(*net.TCPAddr)
	port := tcpAddr.Port

	token, err := GenerateToken()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}

	mgr := session.NewManager(scfg)
	s := &Server{
		cfg:      cfg,
		listener: ln,
		sessions: mgr,
		registry: registry,
		token:    token,
		socket:   cfg.SocketPath,
		logger:   cfg.Logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/call", s.requireAuth(s.handleCall))
	mux.HandleFunc("/v1/list-tools", s.requireAuth(s.handleListTools))
	mux.HandleFunc("/v1/status", s.requireAuth(s.handleStatus))

	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := WriteSocketFile(cfg.SocketPath, SocketInfo{
		Port:  port,
		Token: token,
		PID:   os.Getpid(),
	}); err != nil {
		_ = ln.Close()
		mgr.Close()
		return nil, err
	}

	go func() {
		err := s.server.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(s.logger, "daemon: serve exited: %v\n", err)
		}
	}()

	fmt.Fprintf(s.logger, "daemon: listening on %s (%s)\n", ln.Addr(), platformNote())
	fmt.Fprintf(s.logger, "daemon: socket file %s\n", cfg.SocketPath)
	s.started = true
	return s, nil
}

// Addr returns the actual listener address (useful when Port=0 was passed).
func (s *Server) Addr() net.Addr { return s.listener.Addr() }

// Token returns the bearer token clients must present.
func (s *Server) Token() string { return s.token }

// Stop shuts down the HTTP server, closes all sessions, and removes the
// socket file. Idempotent.
func (s *Server) Stop(ctx context.Context) error {
	s.startMu.Lock()
	if s.stopped {
		s.startMu.Unlock()
		return nil
	}
	s.stopped = true
	s.startMu.Unlock()

	_ = RemoveSocketFile(s.socket)
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.server.Shutdown(shutdownCtx)
	s.sessions.Close()
	return err
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":              true,
		"envelopeVersion": tools.EnvelopeVersion,
	})
}

// requireAuth wraps a handler with bearer-token enforcement. Anything missing
// the right token is rejected with 401 and a generic JSON body — no token
// hints leak.
func (s *Server) requireAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) || auth[len(prefix):] != s.token {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r)
	}
}

// handleCall accepts a CallRequest and runs it through the dispatcher in
// session mode (no Ephemeral — connections persist across requests).
func (s *Server) handleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	var req CallRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode: "+err.Error())
		return
	}
	if req.Tool == "" {
		writeJSONError(w, http.StatusBadRequest, "tool is required")
		return
	}

	ctx := r.Context()
	if req.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	d := tools.NewDispatcher(s.registry, s.sessions)
	env := d.Dispatch(ctx, tools.Request{
		Tool:      req.Tool,
		Params:    req.Params,
		Endpoint:  webview2.Config{WSEndpoint: req.Endpoint.WSEndpoint, BrowserURL: req.Endpoint.BrowserURL},
		SessionID: req.SessionID,
	})
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(env)
}

func (s *Server) handleListTools(w http.ResponseWriter, _ *http.Request) {
	type item struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Schema      json.RawMessage `json:"schema"`
	}
	registered := s.registry.List()
	out := struct {
		EnvelopeVersion string `json:"envelopeVersion"`
		Tools           []item `json:"tools"`
	}{
		EnvelopeVersion: tools.EnvelopeVersion,
		Tools:           make([]item, 0, len(registered)),
	}
	for _, t := range registered {
		out.Tools = append(out.Tools, item{
			Name:        t.Name,
			Description: t.Description,
			Schema:      t.Schema,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"envelopeVersion": tools.EnvelopeVersion,
		"sessions":        s.sessions.Snapshot(),
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

// CallRequest is the JSON body for POST /v1/call.
type CallRequest struct {
	Tool      string          `json:"tool"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Endpoint  EndpointConfig  `json:"endpoint,omitempty"`
	TimeoutMs int             `json:"timeoutMs,omitempty"`
}

// EndpointConfig mirrors webview2.Config for JSON wire use.
type EndpointConfig struct {
	WSEndpoint string `json:"wsEndpoint,omitempty"`
	BrowserURL string `json:"browserUrl,omitempty"`
}
