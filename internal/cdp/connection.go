// Package cdp is a minimal Chrome DevTools Protocol client over a single
// WebSocket. It owns request/response correlation by id, demultiplexes events
// by method, and routes session-scoped messages via the CDP "flatten" model.
package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"

	internallog "github.com/dsbissett/office-addin-mcp/internal/log"
)

// RemoteError is a CDP error response payload.
type RemoteError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *RemoteError) Error() string {
	if e.Data != "" {
		return fmt.Sprintf("cdp: %s (code %d): %s", e.Message, e.Code, e.Data)
	}
	return fmt.Sprintf("cdp: %s (code %d)", e.Message, e.Code)
}

// Event is a CDP event delivered via the read pump.
type Event struct {
	SessionID string
	Method    string
	Params    json.RawMessage
}

type frame struct {
	ID        int64           `json:"id,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *RemoteError    `json:"error,omitempty"`
}

// Connection is one WebSocket to a CDP endpoint. It is safe for concurrent use.
type Connection struct {
	ws         *websocket.Conn
	nextID     atomic.Int64
	roundTrips atomic.Int64

	mu        sync.Mutex
	pending   map[int64]chan frame
	subs      map[string][]chan Event
	closed    bool
	closedErr error

	writeMu sync.Mutex
	done    chan struct{}
}

// ErrClosed is returned for sends after the connection has been closed.
var ErrClosed = errors.New("cdp: connection closed")

// Dial opens a WebSocket to the given CDP URL and starts the read pump.
func Dial(ctx context.Context, wsURL string) (*Connection, error) {
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp dial %q: %w", wsURL, err)
	}
	c := &Connection{
		ws:      ws,
		pending: make(map[int64]chan frame),
		subs:    make(map[string][]chan Event),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// Done returns a channel that is closed once the read pump exits.
func (c *Connection) Done() <-chan struct{} { return c.done }

// Close terminates the connection. Pending sends fail with ErrClosed.
func (c *Connection) Close() error {
	c.closeWithErr(ErrClosed)
	return nil
}

func (c *Connection) closeWithErr(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.closedErr = err
	pending := c.pending
	c.pending = nil
	subs := c.subs
	c.subs = nil
	c.mu.Unlock()

	_ = c.ws.Close()
	for _, ch := range pending {
		close(ch)
	}
	closed := make(map[chan Event]struct{}, len(subs))
	for _, list := range subs {
		for _, ch := range list {
			if _, done := closed[ch]; done {
				continue
			}
			close(ch)
			closed[ch] = struct{}{}
		}
	}
}

func (c *Connection) readLoop() {
	defer close(c.done)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("goroutine panic", "goroutine", "cdp.readLoop", "panic", r)
			c.closeWithErr(fmt.Errorf("cdp readLoop panic: %v", r))
		}
	}()
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			c.closeWithErr(fmt.Errorf("cdp read: %w", err))
			return
		}
		var f frame
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		if f.ID != 0 {
			c.mu.Lock()
			ch, ok := c.pending[f.ID]
			if ok {
				delete(c.pending, f.ID)
			}
			c.mu.Unlock()
			if ok {
				ch <- f
				close(ch)
			}
			continue
		}
		if f.Method != "" {
			c.mu.Lock()
			subs := append([]chan Event(nil), c.subs[f.Method]...)
			c.mu.Unlock()
			ev := Event{SessionID: f.SessionID, Method: f.Method, Params: f.Params}
			for _, s := range subs {
				select {
				case s <- ev:
				default:
				}
			}
		}
	}
}

// SubscribeMethods is the multi-method form of Subscribe: one channel that
// receives events for every method in the list, in the order the read loop
// observes them. Useful when downstream correlation depends on cross-method
// ordering (e.g. Network.requestWillBeSent before Network.responseReceived
// for the same requestId). The returned cancel removes the subscription
// from every method.
func (c *Connection) SubscribeMethods(methods []string, buffer int) (<-chan Event, func()) {
	ch := make(chan Event, buffer)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	for _, m := range methods {
		c.subs[m] = append(c.subs[m], ch)
	}
	c.mu.Unlock()
	cancel := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return
		}
		closed := false
		for _, m := range methods {
			list := c.subs[m]
			for i, s := range list {
				if s == ch {
					c.subs[m] = append(list[:i], list[i+1:]...)
					if !closed {
						close(ch)
						closed = true
					}
					break
				}
			}
		}
	}
	return ch, cancel
}

// Subscribe returns a channel for events of the given method. The returned
// cancel function unsubscribes; the channel may also be closed by Close.
func (c *Connection) Subscribe(method string, buffer int) (<-chan Event, func()) {
	ch := make(chan Event, buffer)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	c.subs[method] = append(c.subs[method], ch)
	c.mu.Unlock()
	cancel := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return
		}
		list := c.subs[method]
		for i, s := range list {
			if s == ch {
				c.subs[method] = append(list[:i], list[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, cancel
}

type outgoing struct {
	ID        int64  `json:"id"`
	SessionID string `json:"sessionId,omitempty"`
	Method    string `json:"method"`
	Params    any    `json:"params,omitempty"`
}

// RoundTrips returns the number of Send calls issued on this connection.
// Used by Diagnostics.CDPRoundTrips to expose session-reuse savings.
func (c *Connection) RoundTrips() int64 { return c.roundTrips.Load() }

// Send issues a CDP command and waits for the matching response. sessionID may
// be empty for browser-level commands; otherwise it routes via flatten sessions.
func (c *Connection) Send(ctx context.Context, sessionID, method string, params any) (json.RawMessage, error) {
	c.roundTrips.Add(1)
	if rid := internallog.RequestID(ctx); rid != "" {
		slog.Debug("cdp.send", "request_id", rid, "session_id", sessionID, "method", method)
	}
	c.mu.Lock()
	if c.closed {
		err := c.closedErr
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: %v", ErrClosed, err)
	}
	id := c.nextID.Add(1)
	ch := make(chan frame, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	cleanup := func() {
		c.mu.Lock()
		if c.pending != nil {
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}

	raw, err := json.Marshal(outgoing{ID: id, SessionID: sessionID, Method: method, Params: params})
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("cdp marshal %s: %w", method, err)
	}

	c.writeMu.Lock()
	err = c.ws.WriteMessage(websocket.TextMessage, raw)
	c.writeMu.Unlock()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("cdp write %s: %w", method, err)
	}

	select {
	case f, ok := <-ch:
		if !ok {
			c.mu.Lock()
			err := c.closedErr
			c.mu.Unlock()
			return nil, fmt.Errorf("%w: %v", ErrClosed, err)
		}
		if f.Error != nil {
			return nil, f.Error
		}
		return f.Result, nil
	case <-ctx.Done():
		cleanup()
		return nil, ctx.Err()
	}
}
