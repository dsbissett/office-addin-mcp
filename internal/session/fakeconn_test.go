package session

import (
	"context"
	"encoding/json"
	"sync"
)

// fakeSender implements Sender by recording every Send call. Used by
// enable-once tests to assert "<Domain>.enable" runs the expected number of
// times across N sequential calls.
type fakeSender struct {
	mu    sync.Mutex
	calls []fakeCall
}

type fakeCall struct {
	SessionID string
	Method    string
}

func (f *fakeSender) Send(_ context.Context, sessionID, method string, _ any) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{SessionID: sessionID, Method: method})
	return json.RawMessage(`{}`), nil
}

// methodCount returns how many times method was sent across all sessions.
func (f *fakeSender) methodCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.Method == method {
			n++
		}
	}
	return n
}
