package powerpointtool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// discoverData is the minimal top-level shape RunDiscover expects: filePath and
// fingerprint at the root, plus arbitrary payload fields.
func discoverData(filePath, fingerprint string) map[string]any {
	return map[string]any{
		"filePath":    filePath,
		"fingerprint": fingerprint,
		"title":       "Deck",
		"slideCount":  float64(3),
	}
}

func TestRunDiscover_RefreshThenCacheHit(t *testing.T) {
	resp := officeOK(discoverData(`C:\decks\report.pptx`, "fp-1"))
	env, _ := fakeEnvWithCache(t, resp)

	// First call: cache miss → refresh, persists the snapshot.
	res1 := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res1.Err != nil {
		t.Fatalf("refresh: unexpected error: %+v", res1.Err)
	}
	if !strings.Contains(res1.Summary, "discovery refreshed") {
		t.Errorf("refresh summary=%q", res1.Summary)
	}
	if m, ok := res1.Data.(map[string]any); !ok || m["cached"] != false {
		t.Errorf("refresh data cached flag = %v", res1.Data)
	}

	// Second call with identical fingerprint → cache hit.
	res2 := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res2.Err != nil {
		t.Fatalf("hit: unexpected error: %+v", res2.Err)
	}
	if !strings.Contains(res2.Summary, "cache hit") {
		t.Errorf("hit summary=%q", res2.Summary)
	}
	m, ok := res2.Data.(map[string]any)
	if !ok || m["cached"] != true {
		t.Errorf("hit data cached flag = %v", res2.Data)
	}
}

func TestRunDiscover_ForceBypassesCache(t *testing.T) {
	resp := officeOK(discoverData(`C:\decks\report.pptx`, "fp-1"))
	env, _ := fakeEnvWithCache(t, resp)

	// Seed the cache.
	if res := runDiscover(context.Background(), json.RawMessage(`{}`), env); res.Err != nil {
		t.Fatalf("seed: %+v", res.Err)
	}

	// force=true must re-run discovery and report a refresh even with a
	// matching fingerprint on disk.
	res := runDiscover(context.Background(), json.RawMessage(`{"force":true}`), env)
	if res.Err != nil {
		t.Fatalf("force: unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("force summary=%q", res.Summary)
	}
}

func TestRunDiscover_RefreshWithoutCache(t *testing.T) {
	// No DocCache wired: Store methods are nil-safe, Get is a miss and Put is a
	// no-op, so the tool reports a fresh refresh.
	env := fakeEnv(t, officeOK(discoverData(`C:\decks\a.pptx`, "fp-x")))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunDiscover_DecodeError(t *testing.T) {
	// Payload returns a JSON array; unmarshaling into the {filePath,fingerprint}
	// head struct fails → decode_discover.
	env, _ := fakeEnvWithCache(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice([]any{1, 2, 3}), nil
	})
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "decode_discover" {
		t.Fatalf("want decode_discover, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category=%q want internal", res.Err.Category)
	}
}

func TestRunDiscover_OfficeError(t *testing.T) {
	env, _ := fakeEnvWithCache(t, officeErr("AccessDenied", "locked"))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "AccessDenied" {
		t.Errorf("err=%+v, want office_js/AccessDenied", res.Err)
	}
}

func TestRunDiscover_AttachFailure(t *testing.T) {
	env := errEnv()
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
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
