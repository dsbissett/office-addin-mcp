package inspecttool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
)

// seedNetwork appends networkRecord JSON to the session's network buffer for
// "cdp-1".
func seedNetwork(t *testing.T, sess *session.Session, kind string, rec networkRecord) {
	t.Helper()
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	sess.EventBuf(session.NetworkBufKind, "cdp-1", defaultMaxBuffer).Append(kind, data)
}

func TestRunNetworkLog_DrainAll(t *testing.T) {
	sess := newSession()
	seedNetwork(t, sess, "network.complete", networkRecord{
		RequestID: "r1", URL: "https://contoso/api", Method: "GET", Status: 200,
		ReqHeaders: json.RawMessage(`{"a":"b"}`), RespHeaders: json.RawMessage(`{"c":"d"}`),
	})
	seedNetwork(t, sess, "network.failed", networkRecord{
		RequestID: "r2", URL: "https://other/x", Method: "POST", Failed: true, ErrorText: "boom",
	})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp:   func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runNetworkLog(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out, ok := res.Data.(networkLogResponse)
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if len(out.Records) != 2 {
		t.Fatalf("records=%d, want 2", len(out.Records))
	}
	// Default omits headers.
	for _, r := range out.Records {
		if r.ReqHeaders != nil || r.RespHeaders != nil {
			t.Errorf("headers should be stripped by default: %+v", r)
		}
	}
}

func TestRunNetworkLog_IncludeHeaders(t *testing.T) {
	sess := newSession()
	seedNetwork(t, sess, "network.complete", networkRecord{
		RequestID: "r1", URL: "https://contoso/api", Status: 200,
		ReqHeaders: json.RawMessage(`{"a":"b"}`),
	})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess: sess,
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runNetworkLog(context.Background(), json.RawMessage(`{"includeHeaders":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(networkLogResponse)
	if len(out.Records) != 1 || out.Records[0].ReqHeaders == nil {
		t.Errorf("headers should be present: %+v", out.Records)
	}
}

func TestRunNetworkLog_FailedOnly(t *testing.T) {
	sess := newSession()
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r1", URL: "u1", Status: 200})
	seedNetwork(t, sess, "network.failed", networkRecord{RequestID: "r2", URL: "u2", Failed: true})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess: sess,
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runNetworkLog(context.Background(), json.RawMessage(`{"failedOnly":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(networkLogResponse)
	if len(out.Records) != 1 || out.Records[0].RequestID != "r2" {
		t.Errorf("failedOnly wrong: %+v", out.Records)
	}
}

func TestRunNetworkLog_StatusRange(t *testing.T) {
	sess := newSession()
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r1", URL: "u1", Status: 200})
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r2", URL: "u2", Status: 404})
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r3", URL: "u3", Status: 500})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess: sess,
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runNetworkLog(context.Background(), json.RawMessage(`{"statusMin":400,"statusMax":499}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(networkLogResponse)
	if len(out.Records) != 1 || out.Records[0].RequestID != "r2" {
		t.Errorf("status range wrong: %+v", out.Records)
	}
	// LastSeq should reflect the highest inspected seq (3), not just the
	// returned tail.
	if out.LastSeq != 3 {
		t.Errorf("lastSeq=%d, want 3 (highest inspected)", out.LastSeq)
	}
}

func TestRunNetworkLog_URLMatchSubstring(t *testing.T) {
	sess := newSession()
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r1", URL: "https://contoso/api/users", Status: 200})
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r2", URL: "https://cdn/asset.js", Status: 200})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess: sess,
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runNetworkLog(context.Background(), json.RawMessage(`{"urlMatch":"contoso"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(networkLogResponse)
	if len(out.Records) != 1 || out.Records[0].RequestID != "r1" {
		t.Errorf("substring urlMatch wrong: %+v", out.Records)
	}
}

func TestRunNetworkLog_URLMatchRegex(t *testing.T) {
	sess := newSession()
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r1", URL: "https://contoso/api/v1", Status: 200})
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r2", URL: "https://contoso/api/v2", Status: 200})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess: sess,
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runNetworkLog(context.Background(), json.RawMessage(`{"urlMatch":"v[12]$"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(networkLogResponse)
	if len(out.Records) != 2 {
		t.Errorf("regex urlMatch should hit both: %+v", out.Records)
	}
}

func TestRunNetworkLog_URLMatchInvalidRegex(t *testing.T) {
	sess := newSession()
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r1", URL: "u", Status: 200})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess: sess,
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runNetworkLog(context.Background(), json.RawMessage(`{"urlMatch":"([a-z"}`), env)
	if res.Err == nil || res.Err.Code != "url_match_invalid" {
		t.Fatalf("want url_match_invalid, got %+v", res.Err)
	}
}

func TestRunNetworkLog_Clear(t *testing.T) {
	sess := newSession()
	seedNetwork(t, sess, "network.complete", networkRecord{RequestID: "r1", URL: "u", Status: 200})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp:   func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runNetworkLog(context.Background(), json.RawMessage(`{"clear":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(networkLogResponse)
	if len(out.Records) != 0 {
		t.Errorf("clear should return no records, got %d", len(out.Records))
	}
	if res.Summary != "Cleared network buffer." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunNetworkLog_AttachFailure(t *testing.T) {
	res := runNetworkLog(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunNetworkLog_BadParams(t *testing.T) {
	res := runNetworkLog(context.Background(), json.RawMessage(`{"limit":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunNetworkLog_PumpEnableFailed(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		enableErr: cdpRemote("Network enable failed"),
		resp:      func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runNetworkLog(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "network_pump_failed" {
		t.Fatalf("want network_pump_failed, got %+v", res.Err)
	}
}

func TestNetworkLog_ToolMetadata(t *testing.T) {
	tool := NetworkLog()
	if tool.Name != "page.networkLog" || tool.Run == nil {
		t.Errorf("unexpected tool metadata: %+v", tool)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}

// --- compileURLMatcher / urlMatcher.match pure-function coverage ---

func TestCompileURLMatcher_Empty(t *testing.T) {
	m, err := compileURLMatcher("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m != nil {
		t.Errorf("empty pattern should yield nil matcher")
	}
	// A nil matcher matches everything.
	if !m.match("anything") {
		t.Errorf("nil matcher should match")
	}
}

func TestCompileURLMatcher_Substring(t *testing.T) {
	// No regex metacharacters (no '.') → treated as a plain substring.
	m, err := compileURLMatcher("contoso/api")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m.re != nil {
		t.Errorf("plain string should not compile as regex")
	}
	if m.subs != "contoso/api" {
		t.Errorf("subs=%q, want contoso/api", m.subs)
	}
	if !m.match("https://contoso/api/x") || m.match("https://nope") {
		t.Errorf("substring match wrong")
	}
}

func TestCompileURLMatcher_Regex(t *testing.T) {
	m, err := compileURLMatcher(`^https://.*\.js$`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m.re == nil {
		t.Errorf("metachar pattern should compile as regex")
	}
	if !m.match("https://x/app.js") || m.match("https://x/app.css") {
		t.Errorf("regex match wrong")
	}
}

func TestCompileURLMatcher_InvalidRegex(t *testing.T) {
	_, err := compileURLMatcher("([a-z")
	if err == nil {
		t.Errorf("expected an error for invalid regex")
	}
}

// --- filterNetworkRecords direct coverage of the unmarshal-skip path ---

func TestFilterNetworkRecords_SkipsBadJSON(t *testing.T) {
	in := []session.EventRecord{
		{Seq: 1, Kind: "network.complete", Data: json.RawMessage(`{bad json`)},
		{Seq: 2, Kind: "network.complete", Data: mustJSON(t, networkRecord{RequestID: "ok", URL: "u", Status: 200})},
	}
	out, lastSeq := filterNetworkRecords(in, networkLogParams{}, nil)
	if len(out) != 1 || out[0].RequestID != "ok" {
		t.Errorf("bad json should be skipped: %+v", out)
	}
	if lastSeq != 2 {
		t.Errorf("lastSeq=%d, want 2", lastSeq)
	}
}

func TestFilterNetworkRecords_URLMismatchSkipped(t *testing.T) {
	m, err := compileURLMatcher("keepme")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	in := []session.EventRecord{
		{Seq: 1, Kind: "network.complete", Data: mustJSON(t, networkRecord{RequestID: "a", URL: "https://drop"})},
		{Seq: 2, Kind: "network.complete", Data: mustJSON(t, networkRecord{RequestID: "b", URL: "https://keepme/x"})},
	}
	out, _ := filterNetworkRecords(in, networkLogParams{}, m)
	if len(out) != 1 || out[0].RequestID != "b" {
		t.Errorf("url filter wrong: %+v", out)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
