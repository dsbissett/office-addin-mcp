package officetool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// fakeEnv returns a RunEnv whose Attach hands the runner a real *cdp.Connection
// backed by an in-process CDP server driven by resp. This is the reusable seam
// for happy-path and Office.js-error coverage of RunPayload / RunDiscover /
// runEmbed.
func fakeEnv(t *testing.T, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
	}
}

// errEnv returns a RunEnv whose Attach always fails with the given error —
// exercises attach-failure branches without a server.
func errEnv(err error) *tools.RunEnv {
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return nil, err
		},
	}
}

// evalException builds a Runtime.evaluate response carrying exceptionDetails so
// the executor returns a *officejs.ProtocolException. cdptest.Eval only sets the
// "result" field, so the protocol-exception path needs this hand-rolled shape.
func evalException(text, description string) any {
	return map[string]any{
		"result": map[string]any{"type": "undefined"},
		"exceptionDetails": map[string]any{
			"exceptionId": 1,
			"text":        text,
			"exception": map[string]any{
				"type":        "object",
				"className":   "Error",
				"description": description,
			},
		},
	}
}

// --- RunPayload --------------------------------------------------------------

func TestRunPayload_HappyPathWithSummary(t *testing.T) {
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice(map[string]any{"address": "A1:B2"}), nil
	})
	var gotData any
	res := RunPayload(context.Background(), env, tools.TargetSelector{}, "excel.readRange",
		map[string]any{"address": "A1:B2"},
		func(data any) string {
			gotData = data
			return "read ok"
		}, "Excel")
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "read ok" {
		t.Errorf("summary=%q, want %q", res.Summary, "read ok")
	}
	m, ok := res.Data.(map[string]any)
	if !ok || m["address"] != "A1:B2" {
		t.Errorf("data=%#v", res.Data)
	}
	if gotData == nil {
		t.Error("summaryFn did not receive decoded data")
	}
}

func TestRunPayload_HappyPathNilSummary(t *testing.T) {
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice([]any{1, 2, 3}), nil
	})
	res := RunPayload(context.Background(), env, tools.TargetSelector{}, "excel.readRange", nil, nil, "Excel")
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "" {
		t.Errorf("expected empty summary, got %q", res.Summary)
	}
	if res.Data == nil {
		t.Error("expected data")
	}
}

func TestRunPayload_OfficeErrorWithDebugInfo(t *testing.T) {
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOfficeErr("ItemNotFound", "Worksheet not found",
			map[string]any{"errorLocation": "Worksheet.getItem"}), nil
	})
	res := RunPayload(context.Background(), env, tools.TargetSelector{}, "excel.readRange", nil, nil, "Excel")
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "ItemNotFound" {
		t.Errorf("err=%+v, want office_js/ItemNotFound", res.Err)
	}
	if res.Summary != "Office.js error: Worksheet not found" {
		t.Errorf("summary=%q", res.Summary)
	}
	di, ok := res.Err.Details["debugInfo"].(map[string]any)
	if !ok || di["errorLocation"] != "Worksheet.getItem" {
		t.Errorf("debugInfo not forwarded: %#v", res.Err.Details)
	}
}

func TestRunPayload_OfficeErrorEmptyCodeDefaults(t *testing.T) {
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOfficeErr("", "boom", nil), nil
	})
	res := RunPayload(context.Background(), env, tools.TargetSelector{}, "excel.readRange", nil, nil, "Excel")
	if res.Err == nil || res.Err.Code != "office_js_error" {
		t.Fatalf("want default office_js_error code, got %+v", res.Err)
	}
	// No debugInfo provided -> Details exists but has no debugInfo key.
	if _, has := res.Err.Details["debugInfo"]; has {
		t.Errorf("did not expect debugInfo, got %#v", res.Err.Details)
	}
}

func TestRunPayload_ProtocolException(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return evalException("Uncaught", "SyntaxError: unexpected token"), nil
		}
		return map[string]any{}, nil
	})
	res := RunPayload(context.Background(), env, tools.TargetSelector{}, "excel.readRange", nil, nil, "Excel")
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Code != "payload_protocol_exception" || res.Err.Category != tools.CategoryProtocol {
		t.Errorf("err=%+v, want payload_protocol_exception/protocol", res.Err)
	}
	if !strings.Contains(res.Summary, "Payload protocol exception:") {
		t.Errorf("summary=%q", res.Summary)
	}
	if !strings.Contains(res.Err.Message, "SyntaxError") {
		t.Errorf("message=%q, want exception description", res.Err.Message)
	}
}

func TestRunPayload_AttachFailureGeneric(t *testing.T) {
	res := RunPayload(context.Background(), errEnv(errors.New("no target")),
		tools.TargetSelector{}, "excel.readRange", nil, nil, "Excel")
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category=%q, want not_found", res.Err.Category)
	}
	if res.Summary != "Excel attach failed: no target" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunPayload_AttachFailureDeadline(t *testing.T) {
	res := RunPayload(context.Background(), errEnv(context.DeadlineExceeded),
		tools.TargetSelector{}, "excel.readRange", nil, nil, "Excel")
	if res.Err == nil {
		t.Fatal("expected error")
	}
	// ClassifyCDPErr maps DeadlineExceeded to timeout/timeout.
	if res.Err.Category != tools.CategoryTimeout || res.Err.Code != "timeout" {
		t.Errorf("err=%+v, want timeout/timeout", res.Err)
	}
	if !strings.HasPrefix(res.Summary, "Excel attach failed:") {
		t.Errorf("summary=%q", res.Summary)
	}
	if !res.Err.Retryable {
		t.Error("timeout should be retryable")
	}
}

func TestRunPayload_AttachFailureCanceled(t *testing.T) {
	res := RunPayload(context.Background(), errEnv(context.Canceled),
		tools.TargetSelector{}, "excel.readRange", nil, nil, "Excel")
	if res.Err == nil || res.Err.Code != "canceled" {
		t.Fatalf("want canceled, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category=%q, want internal", res.Err.Category)
	}
}

func TestRunPayload_PayloadFailedRemoteError(t *testing.T) {
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return nil, &cdp.RemoteError{Code: -32000, Message: "Execution context destroyed"}
	})
	res := RunPayload(context.Background(), env, tools.TargetSelector{}, "excel.readRange", nil, nil, "Excel")
	if res.Err == nil || res.Err.Code != "payload_failed" {
		t.Fatalf("want payload_failed, got %+v", res.Err)
	}
	if !strings.HasPrefix(res.Summary, "Excel payload failed:") {
		t.Errorf("summary=%q", res.Summary)
	}
	// ClassifyCDPErr surfaces the structured CDP error under Details["cdpError"].
	if _, ok := res.Err.Details["cdpError"]; !ok {
		t.Errorf("expected cdpError details, got %#v", res.Err.Details)
	}
}

func TestRunPayload_UnknownPayloadName(t *testing.T) {
	// getPayload fails before any CDP round-trip; surfaces as payload_failed
	// (not OfficeError / ProtocolException).
	env := fakeEnv(t, nil)
	res := RunPayload(context.Background(), env, tools.TargetSelector{},
		"excel.totallyNotAPayload", nil, nil, "Excel")
	if res.Err == nil || res.Err.Code != "payload_failed" {
		t.Fatalf("want payload_failed, got %+v", res.Err)
	}
	if !strings.Contains(res.Err.Message, "no payload for tool") {
		t.Errorf("message=%q", res.Err.Message)
	}
}

func TestCodeOrDefault(t *testing.T) {
	if got := codeOrDefault(""); got != "office_js_error" {
		t.Errorf("codeOrDefault(\"\")=%q, want office_js_error", got)
	}
	if got := codeOrDefault("ItemNotFound"); got != "ItemNotFound" {
		t.Errorf("codeOrDefault(ItemNotFound)=%q", got)
	}
}

func TestSelectorFields_Selector(t *testing.T) {
	sf := SelectorFields{TargetID: "tid", URLPattern: "taskpane"}
	sel := sf.Selector()
	if sel.TargetID != "tid" || sel.URLPattern != "taskpane" {
		t.Errorf("Selector()=%+v", sel)
	}
	// Zero value yields empty selector.
	empty := SelectorFields{}.Selector()
	if empty.TargetID != "" || empty.URLPattern != "" {
		t.Errorf("empty Selector()=%+v", empty)
	}
}

// --- RunDiscover -------------------------------------------------------------

// discoverEnv wires a fakeEnv with an enabled (in-memory backed) DocCache rooted
// at a unique temp file so cache hits/misses can be exercised.
func discoverEnv(t *testing.T, resp cdptest.Responder) (*tools.RunEnv, *doccache.Store) {
	t.Helper()
	env := fakeEnv(t, resp)
	cachePath := t.TempDir() + "/doccache.json"
	store := doccache.Open(cachePath, false)
	env.DocCache = store
	return env, store
}

func TestRunDiscover_RefreshThenCacheHit(t *testing.T) {
	resp := func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"filePath":    "C:/wb.xlsx",
				"fingerprint": "fp-1",
				"sheets":      []any{"Sheet1", "Sheet2"},
			}), nil
		}
		return map[string]any{}, nil
	}
	env, _ := discoverEnv(t, resp)

	// First call: cache miss -> refresh and persist.
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err != nil {
		t.Fatalf("first discover errored: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("summary=%q, want refreshed", res.Summary)
	}
	m := res.Data.(map[string]any)
	if m["cached"] != false {
		t.Errorf("expected cached=false on refresh, got %#v", m["cached"])
	}
	if m["filePath"] != "C:/wb.xlsx" || m["fingerprint"] != "fp-1" {
		t.Errorf("cache meta missing: %#v", m)
	}
	if _, ok := m["sheets"]; !ok {
		t.Errorf("expected merged payload fields, got %#v", m)
	}

	// Second call: same fingerprint, force=false -> cache hit (returns stored data).
	res2 := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res2.Err != nil {
		t.Fatalf("second discover errored: %+v", res2.Err)
	}
	if !strings.Contains(res2.Summary, "cache hit") {
		t.Errorf("summary=%q, want cache hit", res2.Summary)
	}
	m2 := res2.Data.(map[string]any)
	if m2["cached"] != true {
		t.Errorf("expected cached=true on hit, got %#v", m2["cached"])
	}
}

func TestRunDiscover_ForceBypassesCache(t *testing.T) {
	resp := func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"filePath":    "C:/wb.xlsx",
				"fingerprint": "fp-1",
			}), nil
		}
		return map[string]any{}, nil
	}
	env, _ := discoverEnv(t, resp)

	if res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel"); res.Err != nil {
		t.Fatalf("seed discover errored: %+v", res.Err)
	}
	// force=true must refresh even though the fingerprint matches.
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", true, "Excel")
	if res.Err != nil {
		t.Fatalf("forced discover errored: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("summary=%q, want refreshed under force", res.Summary)
	}
	if res.Data.(map[string]any)["cached"] != false {
		t.Errorf("force should yield cached=false")
	}
}

func TestRunDiscover_FingerprintDriftRefreshes(t *testing.T) {
	fp := "fp-1"
	resp := func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"filePath":    "C:/wb.xlsx",
				"fingerprint": fp,
			}), nil
		}
		return map[string]any{}, nil
	}
	env, _ := discoverEnv(t, resp)

	if res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel"); res.Err != nil {
		t.Fatalf("seed discover errored: %+v", res.Err)
	}
	// Change the live fingerprint: even with force=false this is a refresh.
	fp = "fp-2"
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err != nil {
		t.Fatalf("drift discover errored: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("summary=%q, want refreshed on drift", res.Summary)
	}
	if res.Data.(map[string]any)["fingerprint"] != "fp-2" {
		t.Errorf("expected fp-2 after drift")
	}
}

func TestRunDiscover_DisabledCacheAlwaysRefreshes(t *testing.T) {
	resp := func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"filePath":    "C:/wb.xlsx",
				"fingerprint": "fp-1",
			}), nil
		}
		return map[string]any{}, nil
	}
	env := fakeEnv(t, resp)
	env.DocCache = doccache.Open("", true) // disabled store: Get miss, Put no-op

	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err != nil {
		t.Fatalf("discover errored: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("summary=%q, want refreshed", res.Summary)
	}
	// Second call still a refresh because disabled store never caches.
	res2 := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if !strings.Contains(res2.Summary, "discovery refreshed") {
		t.Errorf("disabled cache should never hit; summary=%q", res2.Summary)
	}
}

func TestRunDiscover_NonCacheableFilePathRefreshes(t *testing.T) {
	// Empty filePath is non-cacheable (Get miss + Put no-op): always a refresh,
	// and Put returns nil so we hit the trailing refresh branch.
	resp := func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"filePath":    "",
				"fingerprint": "fp-1",
			}), nil
		}
		return map[string]any{}, nil
	}
	env, _ := discoverEnv(t, resp)
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err != nil {
		t.Fatalf("discover errored: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("summary=%q, want refreshed", res.Summary)
	}
}

func TestRunDiscover_CacheWriteFailure(t *testing.T) {
	// Point the cache at a path whose parent component is a regular file, so
	// saveLocked's MkdirAll fails -> Put returns an error -> RunDiscover takes
	// the "cache write failed" branch but still returns OK with the live data.
	dir := t.TempDir()
	blocker := dir + "/blocker"
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"filePath":    "C:/wb.xlsx",
				"fingerprint": "fp-1",
				"sheets":      []any{"Sheet1"},
			}), nil
		}
		return map[string]any{}, nil
	})
	// blocker is a file, so "blocker/nested/doccache.json" cannot be created.
	env.DocCache = doccache.Open(blocker+"/nested/doccache.json", false)

	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err != nil {
		t.Fatalf("expected OK with cache-write-failed note, got err %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "cache write failed") {
		t.Errorf("summary=%q, want cache write failed note", res.Summary)
	}
	m := res.Data.(map[string]any)
	if m["cached"] != false {
		t.Errorf("expected cached=false, got %#v", m["cached"])
	}
	if _, ok := m["sheets"]; !ok {
		t.Errorf("expected live payload data merged, got %#v", m)
	}
}

func TestRunDiscover_AttachFailure(t *testing.T) {
	env := errEnv(errors.New("no target"))
	env.DocCache = doccache.Open("", true)
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category=%q", res.Err.Category)
	}
	if res.Summary != "Excel attach failed: no target" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunDiscover_DecodeHeadFailure(t *testing.T) {
	// Payload "result" is a JSON array, not an object -> head unmarshal fails.
	env, _ := discoverEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice([]any{"not", "an", "object"}), nil
		}
		return map[string]any{}, nil
	})
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err == nil || res.Err.Code != "decode_discover" {
		t.Fatalf("want decode_discover, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunDiscover_OfficeError(t *testing.T) {
	env, _ := discoverEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOfficeErr("AccessDenied", "no access", map[string]any{"x": 1}), nil
		}
		return map[string]any{}, nil
	})
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "AccessDenied" {
		t.Fatalf("want office_js/AccessDenied, got %+v", res.Err)
	}
	if res.Summary != "Office.js error: no access" {
		t.Errorf("summary=%q", res.Summary)
	}
}

// Note: the decode_cached branch in RunDiscover requires cached.Data to be
// syntactically invalid JSON. doccache only ever stores Data that round-trips
// through json.Marshal/json.Unmarshal (Put marshals the whole file; load reads
// it back as a json.RawMessage extracted from a valid outer document), so a
// corrupt Data value cannot be produced through the public store API. That
// branch is therefore defensively unreachable from tests without touching
// production code; see blockers.

// --- withCacheMeta -----------------------------------------------------------

func TestWithCacheMeta_ObjectMerge(t *testing.T) {
	out := withCacheMeta(map[string]any{"sheets": []any{"S1"}}, "C:/f.xlsx", "fp", true)
	if out["sheets"] == nil {
		t.Errorf("expected merged sheets key, got %#v", out)
	}
	if out["cached"] != true || out["filePath"] != "C:/f.xlsx" || out["fingerprint"] != "fp" {
		t.Errorf("meta wrong: %#v", out)
	}
}

func TestWithCacheMeta_NonObjectWrapped(t *testing.T) {
	out := withCacheMeta([]any{1, 2}, "C:/f.xlsx", "fp", false)
	if out["data"] == nil {
		t.Errorf("non-object data should be wrapped under data, got %#v", out)
	}
	if out["cached"] != false {
		t.Errorf("cached wrong: %#v", out)
	}
}

// --- classifyDiscoverErr -----------------------------------------------------

func TestClassifyDiscoverErr_ProtocolException(t *testing.T) {
	// Drive a protocol exception through RunDiscover so classifyDiscoverErr's
	// ProtocolException branch is exercised end to end.
	env, _ := discoverEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return evalException("Uncaught", "TypeError: bad"), nil
		}
		return map[string]any{}, nil
	})
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err == nil || res.Err.Code != "payload_protocol_exception" {
		t.Fatalf("want payload_protocol_exception, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryProtocol {
		t.Errorf("category=%q", res.Err.Category)
	}
	if !strings.Contains(res.Summary, "Payload protocol exception:") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestClassifyDiscoverErr_OfficeErrorEmptyCode(t *testing.T) {
	env, _ := discoverEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOfficeErr("", "boom", nil), nil
		}
		return map[string]any{}, nil
	})
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err == nil || res.Err.Code != "office_js_error" {
		t.Fatalf("want default office_js_error, got %+v", res.Err)
	}
}

func TestClassifyDiscoverErr_PayloadFailedRemoteError(t *testing.T) {
	env, _ := discoverEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return nil, &cdp.RemoteError{Code: -32000, Message: "context gone"}
	})
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.discover", false, "Excel")
	if res.Err == nil || res.Err.Code != "payload_failed" {
		t.Fatalf("want payload_failed, got %+v", res.Err)
	}
	if !strings.HasPrefix(res.Summary, "Excel payload failed:") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunDiscover_UnknownPayloadName(t *testing.T) {
	env, _ := discoverEnv(t, nil)
	res := RunDiscover(context.Background(), env, tools.TargetSelector{}, "excel", "excel.notReal", false, "Excel")
	if res.Err == nil || res.Err.Code != "payload_failed" {
		t.Fatalf("want payload_failed for unknown payload, got %+v", res.Err)
	}
}
