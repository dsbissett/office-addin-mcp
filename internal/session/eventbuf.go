package session

import (
	"encoding/json"
	"sync"
	"time"
)

// EventRecord is one entry in an EventBuf. Seq is monotonic per buffer and
// callers use it as a cursor; Time is unix milliseconds at append.
type EventRecord struct {
	Seq  int64           `json:"seq"`
	Time int64           `json:"time"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// EventBuf is a bounded ring of EventRecords. Safe for concurrent use.
// The pump goroutine that owns Append runs independently of the tool calls
// that drain via Drain.
type EventBuf struct {
	mu      sync.Mutex
	ring    []EventRecord
	max     int
	nextSeq int64
	pumping bool
}

// DrainOpts controls a Drain call.
type DrainOpts struct {
	SinceSeq int64
	Limit    int
	Peek     bool
}

// DrainResult is what Drain returns.
type DrainResult struct {
	Records []EventRecord `json:"records"`
	LastSeq int64         `json:"lastSeq"`
	Dropped bool          `json:"dropped"`
}

func newEventBuf(max int) *EventBuf {
	if max <= 0 {
		max = 1000
	}
	return &EventBuf{max: max}
}

// SetMax resizes the buffer, dropping oldest entries when shrinking.
func (b *EventBuf) SetMax(max int) {
	if max <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.max = max
	if len(b.ring) > max {
		b.ring = append([]EventRecord(nil), b.ring[len(b.ring)-max:]...)
	}
}

// Max returns the current capacity.
func (b *EventBuf) Max() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.max
}

// Append adds a record and returns its assigned seq. Drops oldest on overflow.
func (b *EventBuf) Append(kind string, data json.RawMessage) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextSeq++
	rec := EventRecord{
		Seq:  b.nextSeq,
		Time: time.Now().UnixMilli(),
		Kind: kind,
		Data: data,
	}
	if len(b.ring) >= b.max {
		// drop oldest
		copy(b.ring, b.ring[1:])
		b.ring[len(b.ring)-1] = rec
	} else {
		b.ring = append(b.ring, rec)
	}
	return rec.Seq
}

// Drain returns records with Seq > opts.SinceSeq, capped at opts.Limit.
// Dropped is true when sinceSeq points before the oldest retained entry —
// callers should treat that as a gap. Peek=true does not advance any state
// (the cursor is purely caller-supplied).
func (b *EventBuf) Drain(opts DrainOpts) DrainResult {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := DrainResult{Records: []EventRecord{}}
	if len(b.ring) == 0 {
		out.LastSeq = b.nextSeq
		return out
	}
	oldest := b.ring[0].Seq
	if opts.SinceSeq > 0 && opts.SinceSeq < oldest-1 {
		out.Dropped = true
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = len(b.ring)
	}
	for _, r := range b.ring {
		if r.Seq <= opts.SinceSeq {
			continue
		}
		out.Records = append(out.Records, r)
		if len(out.Records) >= limit {
			break
		}
	}
	if len(out.Records) > 0 {
		out.LastSeq = out.Records[len(out.Records)-1].Seq
	} else {
		out.LastSeq = opts.SinceSeq
	}
	return out
}

// Clear empties the ring without resetting the seq counter — callers using
// sinceSeq as a cursor see "no new entries" rather than time-traveling.
func (b *EventBuf) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ring = nil
}

// markPumpingLocked is the underlying check-and-set used by Session.
// Returns true when the caller is responsible for starting the pump.
func (b *EventBuf) markPumpingLocked() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pumping {
		return false
	}
	b.pumping = true
	return true
}

// Event-buffer accessors on Session.
//
// Buffers are keyed by (kind, cdpSessionID). Per-target by construction:
// each target has its own cdpSessionID, so navigating between pages with
// pages.select preserves the buffer for the previous target. All buffers
// are dropped when the underlying CDP connection is reconnected, since
// cdpSessionIDs do not survive the new socket anyway.

// EventBufKind enumerates the event-buffer streams.
type EventBufKind string

const (
	// ConsoleBufKind buffers Runtime.consoleAPICalled / Runtime.exceptionThrown
	// / Log.entryAdded events.
	ConsoleBufKind EventBufKind = "console"
	// NetworkBufKind buffers correlated Network.* request lifecycles.
	NetworkBufKind EventBufKind = "network"
)

type bufKey struct {
	kind   EventBufKind
	cdpSID string
}

// EventBuf returns the buffer for (kind, cdpSessionID), creating it with
// the supplied max on first access. Existing buffers honor the new max via
// SetMax. Self-locking on eventMu — callers do not need to hold any other
// session lock.
func (s *Session) EventBuf(kind EventBufKind, cdpSessionID string, max int) *EventBuf {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if s.eventBufs == nil {
		s.eventBufs = map[bufKey]*EventBuf{}
	}
	k := bufKey{kind: kind, cdpSID: cdpSessionID}
	buf, ok := s.eventBufs[k]
	if !ok {
		buf = newEventBuf(max)
		s.eventBufs[k] = buf
		return buf
	}
	if max > 0 && max != buf.Max() {
		buf.SetMax(max)
	}
	return buf
}

// MarkEventPumping atomically checks whether a pump is already running for
// (kind, cdpSessionID) and, if not, marks it as running. Returns true when
// the caller should start the pump goroutine. Self-locking.
func (s *Session) MarkEventPumping(kind EventBufKind, cdpSessionID string, max int) bool {
	buf := s.EventBuf(kind, cdpSessionID, max)
	return buf.markPumpingLocked()
}
