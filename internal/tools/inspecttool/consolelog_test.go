package inspecttool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
)

// seedConsole appends console records to the session's console buffer for the
// session id the harness uses ("cdp-1").
func seedConsole(sess *session.Session, kinds ...string) {
	buf := sess.EventBuf(session.ConsoleBufKind, "cdp-1", defaultMaxBuffer)
	for _, k := range kinds {
		buf.Append(k, json.RawMessage(`{"text":"x"}`))
	}
}

func TestRunConsoleLog_DrainAll(t *testing.T) {
	sess := newSession()
	seedConsole(sess, "console.log", "console.warn", "exception")
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp:   func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runConsoleLog(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out, ok := res.Data.(consoleLogResponse)
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if len(out.Records) != 3 {
		t.Errorf("records=%d, want 3", len(out.Records))
	}
	if out.TargetID != "T-1" {
		t.Errorf("targetId=%q", out.TargetID)
	}
	if out.Capacity != defaultMaxBuffer {
		t.Errorf("capacity=%d, want %d", out.Capacity, defaultMaxBuffer)
	}
	if !strings.Contains(res.Summary, "Drained 3 console record") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunConsoleLog_LevelFilterShorthandAndExact(t *testing.T) {
	sess := newSession()
	seedConsole(sess, "console.log", "console.warn", "console.error", "exception", "log.entry")
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess: sess,
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	// "warn" shorthand → console.warn; "exception" exact match.
	res := runConsoleLog(context.Background(), json.RawMessage(`{"levels":["warn","exception"]}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(consoleLogResponse)
	if len(out.Records) != 2 {
		t.Fatalf("records=%d, want 2", len(out.Records))
	}
	kinds := map[string]bool{}
	for _, r := range out.Records {
		kinds[r.Kind] = true
	}
	if !kinds["console.warn"] || !kinds["exception"] {
		t.Errorf("filtered kinds wrong: %v", kinds)
	}
}

func TestRunConsoleLog_Clear(t *testing.T) {
	sess := newSession()
	seedConsole(sess, "console.log", "console.warn")
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp:   func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runConsoleLog(context.Background(), json.RawMessage(`{"clear":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(consoleLogResponse)
	if len(out.Records) != 0 {
		t.Errorf("clear should return no records, got %d", len(out.Records))
	}
	if res.Summary != "Cleared console buffer." {
		t.Errorf("summary=%q", res.Summary)
	}
	// Buffer should actually be empty now.
	drained := sess.EventBuf(session.ConsoleBufKind, "cdp-1", defaultMaxBuffer).Drain(session.DrainOpts{})
	if len(drained.Records) != 0 {
		t.Errorf("buffer not cleared: %d records remain", len(drained.Records))
	}
}

func TestRunConsoleLog_AttachFailure(t *testing.T) {
	res := runConsoleLog(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunConsoleLog_BadParams(t *testing.T) {
	res := runConsoleLog(context.Background(), json.RawMessage(`{"limit":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunConsoleLog_PumpEnableFailed(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		enableErr: cdpRemote("Runtime enable failed"),
		resp:      func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runConsoleLog(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "console_pump_failed" {
		t.Fatalf("want console_pump_failed, got %+v", res.Err)
	}
}

func TestConsoleLog_ToolMetadata(t *testing.T) {
	tool := ConsoleLog()
	if tool.Name != "page.consoleLog" || tool.Run == nil {
		t.Errorf("unexpected tool metadata: %+v", tool)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}

// --- filterConsoleLevels pure-function coverage ---

func recs(kinds ...string) []session.EventRecord {
	out := make([]session.EventRecord, len(kinds))
	for i, k := range kinds {
		out[i] = session.EventRecord{Seq: int64(i + 1), Kind: k}
	}
	return out
}

func TestFilterConsoleLevels_EmptyReturnsAll(t *testing.T) {
	in := recs("console.log", "exception")
	got := filterConsoleLevels(in, nil)
	if len(got) != 2 {
		t.Errorf("empty levels should return all, got %d", len(got))
	}
}

func TestFilterConsoleLevels_ExactAndShorthandAndMiss(t *testing.T) {
	in := recs("console.log", "console.warn", "exception", "log.entry")
	// "log" shorthand → console.log; "exception" exact; "nope" matches nothing.
	got := filterConsoleLevels(in, []string{"LOG", "exception", "nope"})
	kinds := map[string]bool{}
	for _, r := range got {
		kinds[r.Kind] = true
	}
	if !kinds["console.log"] || !kinds["exception"] {
		t.Errorf("expected console.log + exception, got %v", kinds)
	}
	if kinds["console.warn"] || kinds["log.entry"] {
		t.Errorf("unexpected kinds leaked: %v", kinds)
	}
}

func TestFilterConsoleLevels_ExactConsoleDotKind(t *testing.T) {
	in := recs("console.error", "console.log")
	got := filterConsoleLevels(in, []string{"console.error"})
	if len(got) != 1 || got[0].Kind != "console.error" {
		t.Errorf("exact console.error filter wrong: %+v", got)
	}
}
