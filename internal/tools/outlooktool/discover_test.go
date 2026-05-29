package outlooktool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// discoverData is a discover payload result with the cache head fields plus
// some host-specific body.
func discoverData(filePath, fingerprint string) map[string]any {
	return map[string]any{
		"filePath":    filePath,
		"fingerprint": fingerprint,
		"userProfile": map[string]any{"emailAddress": "u@example.com"},
		"hostMode":    "messageCompose",
	}
}

func dataMap(t *testing.T, res tools.Result) map[string]any {
	t.Helper()
	m, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data is not a map: %T (%+v)", res.Data, res.Data)
	}
	return m
}

func TestRunDiscover_RefreshAndCachePersist(t *testing.T) {
	env := fakeEnvWithCache(t, officeReply(discoverData("u@example.com", "fp1")))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Outlook discovery refreshed (u@example.com)." {
		t.Errorf("summary=%q", res.Summary)
	}
	m := dataMap(t, res)
	if m["cached"] != false {
		t.Errorf("cached=%v want false", m["cached"])
	}
	if m["filePath"] != "u@example.com" || m["fingerprint"] != "fp1" {
		t.Errorf("cache meta missing: %+v", m)
	}
	if m["hostMode"] != "messageCompose" {
		t.Errorf("host body lost: %+v", m)
	}
}

func TestRunDiscover_CacheHit(t *testing.T) {
	env := fakeEnvWithCache(t, officeReply(discoverData("u@example.com", "fp1")))
	// First call populates the cache.
	if res := runDiscover(context.Background(), json.RawMessage(`{}`), env); res.Err != nil {
		t.Fatalf("seed call failed: %+v", res.Err)
	}
	// Second call with the same fingerprint hits the cache.
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Outlook discovery cache hit (u@example.com)." {
		t.Errorf("summary=%q", res.Summary)
	}
	m := dataMap(t, res)
	if m["cached"] != true {
		t.Errorf("cached=%v want true", m["cached"])
	}
}

func TestRunDiscover_ForceBypassesCache(t *testing.T) {
	env := fakeEnvWithCache(t, officeReply(discoverData("u@example.com", "fp1")))
	if res := runDiscover(context.Background(), json.RawMessage(`{}`), env); res.Err != nil {
		t.Fatalf("seed call failed: %+v", res.Err)
	}
	// force=true re-runs discovery even though the fingerprint matches.
	res := runDiscover(context.Background(), json.RawMessage(`{"force":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Outlook discovery refreshed (u@example.com)." {
		t.Errorf("summary=%q want refreshed", res.Summary)
	}
	if dataMap(t, res)["cached"] != false {
		t.Error("force should report cached=false")
	}
}

// A changed fingerprint is a cache miss → refresh, even without force.
func TestRunDiscover_FingerprintDriftRefreshes(t *testing.T) {
	srv := &driftResponder{fp: "fp1"}
	env := fakeEnvWithCache(t, srv.respond)
	if res := runDiscover(context.Background(), json.RawMessage(`{}`), env); res.Err != nil {
		t.Fatalf("seed call failed: %+v", res.Err)
	}
	srv.fp = "fp2" // document changed
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Outlook discovery refreshed (u@example.com)." {
		t.Errorf("summary=%q want refreshed on drift", res.Summary)
	}
}

type driftResponder struct{ fp string }

func (d *driftResponder) respond(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
	if method == "Runtime.evaluate" {
		return cdptest.EvalOffice(discoverData("u@example.com", d.fp)), nil
	}
	return map[string]any{}, nil
}

func TestRunDiscover_OfficeError(t *testing.T) {
	env := fakeEnvWithCache(t, officeErrReply("UnexpectedError", "mailbox unavailable", nil))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "UnexpectedError" {
		t.Fatalf("want office_js/UnexpectedError, got %+v", res.Err)
	}
}

func TestRunDiscover_AttachFailure(t *testing.T) {
	res := runDiscover(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunDiscover_BadParams(t *testing.T) {
	res := runDiscover(context.Background(), json.RawMessage(`{"force":"yes"}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// A discover payload whose head fields can't be decoded (filePath is a number)
// surfaces decode_discover.
func TestRunDiscover_DecodeHeadFailure(t *testing.T) {
	env := fakeEnvWithCache(t, officeReply(map[string]any{"filePath": 123, "fingerprint": "fp1"}))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "decode_discover" {
		t.Fatalf("want decode_discover, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category=%q want internal", res.Err.Category)
	}
}

// Empty filePath bypasses the cache by design (cacheable() rejects it). The
// discovery still refreshes and reports cached=false.
func TestRunDiscover_EmptyFilePathBypassesCache(t *testing.T) {
	env := fakeEnvWithCache(t, officeReply(discoverData("", "fp1")))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if dataMap(t, res)["cached"] != false {
		t.Error("empty filePath should never report a cache hit")
	}
	// A second call is still a refresh (nothing was persisted).
	res2 := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if dataMap(t, res2)["cached"] != false {
		t.Error("empty filePath must not be cached across calls")
	}
}

func TestDiscoverTool_RunWiring(t *testing.T) {
	env := fakeEnvWithCache(t, officeReply(discoverData("u@example.com", "fp1")))
	res := Discover().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}
