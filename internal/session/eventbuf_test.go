package session

import (
	"encoding/json"
	"testing"
)

func mustData(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestEventBuf_AppendAndDrain(t *testing.T) {
	b := newEventBuf(8)
	for i := 0; i < 5; i++ {
		b.Append("console.log", mustData(t, i))
	}
	res := b.Drain(DrainOpts{})
	if len(res.Records) != 5 {
		t.Fatalf("expected 5 records, got %d", len(res.Records))
	}
	if res.LastSeq != 5 {
		t.Fatalf("expected lastSeq=5, got %d", res.LastSeq)
	}
	if res.Records[0].Seq != 1 {
		t.Errorf("expected first seq=1, got %d", res.Records[0].Seq)
	}
}

func TestEventBuf_DrainSinceSeqAdvances(t *testing.T) {
	b := newEventBuf(8)
	for i := 0; i < 5; i++ {
		b.Append("k", mustData(t, i))
	}
	first := b.Drain(DrainOpts{SinceSeq: 0, Limit: 2})
	if len(first.Records) != 2 || first.LastSeq != 2 {
		t.Fatalf("first drain: %+v", first)
	}
	second := b.Drain(DrainOpts{SinceSeq: first.LastSeq})
	if len(second.Records) != 3 || second.LastSeq != 5 {
		t.Fatalf("second drain: %+v", second)
	}
	// idempotent: drain again with the same cursor returns nothing
	third := b.Drain(DrainOpts{SinceSeq: second.LastSeq})
	if len(third.Records) != 0 {
		t.Errorf("third drain returned %d records, want 0", len(third.Records))
	}
}

func TestEventBuf_OverflowDropsOldest(t *testing.T) {
	b := newEventBuf(3)
	for i := 0; i < 5; i++ {
		b.Append("k", mustData(t, i))
	}
	res := b.Drain(DrainOpts{})
	if len(res.Records) != 3 {
		t.Fatalf("expected 3 retained, got %d", len(res.Records))
	}
	if res.Records[0].Seq != 3 || res.Records[2].Seq != 5 {
		t.Errorf("ring kept wrong window: seqs=%d..%d", res.Records[0].Seq, res.Records[2].Seq)
	}
}

func TestEventBuf_DroppedFlagWhenCursorTooOld(t *testing.T) {
	b := newEventBuf(2)
	for i := 0; i < 5; i++ {
		b.Append("k", mustData(t, i))
	}
	// oldest retained is seq=4. Cursor at 1 is way behind.
	res := b.Drain(DrainOpts{SinceSeq: 1})
	if !res.Dropped {
		t.Errorf("expected Dropped=true when sinceSeq predates ring window")
	}
}

func TestEventBuf_SetMaxShrinks(t *testing.T) {
	b := newEventBuf(10)
	for i := 0; i < 10; i++ {
		b.Append("k", mustData(t, i))
	}
	b.SetMax(3)
	res := b.Drain(DrainOpts{})
	if len(res.Records) != 3 {
		t.Fatalf("expected 3 after shrink, got %d", len(res.Records))
	}
	if res.Records[0].Seq != 8 {
		t.Errorf("expected newest 3 retained, oldest seq=%d", res.Records[0].Seq)
	}
}

func TestEventBuf_ClearKeepsCursor(t *testing.T) {
	b := newEventBuf(8)
	for i := 0; i < 3; i++ {
		b.Append("k", mustData(t, i))
	}
	b.Clear()
	b.Append("k", mustData(t, "after"))
	res := b.Drain(DrainOpts{})
	if len(res.Records) != 1 {
		t.Fatalf("expected 1 record after clear+append, got %d", len(res.Records))
	}
	if res.Records[0].Seq != 4 {
		t.Errorf("seq counter must persist across Clear; got seq=%d", res.Records[0].Seq)
	}
}

func TestEventBuf_MarkPumpingOnce(t *testing.T) {
	b := newEventBuf(4)
	if !b.markPumpingLocked() {
		t.Fatal("first markPumping should claim the slot")
	}
	if b.markPumpingLocked() {
		t.Error("second markPumping should report already running")
	}
}

func TestSession_EventBufPerCdpSession(t *testing.T) {
	s := &Session{cfg: Config{}.withDefaults()}
	a := s.EventBuf(ConsoleBufKind, "sess-A", 100)
	b := s.EventBuf(ConsoleBufKind, "sess-B", 100)
	if a == b {
		t.Fatal("buffers for distinct cdp sessions must not alias")
	}
	a.Append("k", mustData(t, "x"))
	if got := s.EventBuf(ConsoleBufKind, "sess-A", 100); got != a {
		t.Error("repeat lookup should return same buffer")
	}
	if r := b.Drain(DrainOpts{}); len(r.Records) != 0 {
		t.Errorf("buffer B should be empty, got %d", len(r.Records))
	}
}

func TestSession_dropConnLockedClearsEventBufs(t *testing.T) {
	s := &Session{cfg: Config{}.withDefaults()}
	buf := s.EventBuf(NetworkBufKind, "sess-A", 50)
	buf.Append("k", mustData(t, "x"))
	s.dropConnLocked()
	if s.eventBufs != nil {
		t.Errorf("eventBufs map should be nil after dropConnLocked, got %d entries", len(s.eventBufs))
	}
}
