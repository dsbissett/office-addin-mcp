package onenotetool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// discoverPayload is the minimal Office.js discover result: it must carry
// filePath + fingerprint at the top level for RunDiscover to key the cache.
func discoverData(filePath, fingerprint string, extra map[string]any) map[string]any {
	d := map[string]any{"filePath": filePath, "fingerprint": fingerprint}
	for k, v := range extra {
		d[k] = v
	}
	return d
}

func TestDiscover_Refresh_CacheMiss(t *testing.T) {
	env := fakeEnvWithCache(t, okOffice(discoverData("Notebook1.one", "fp1",
		map[string]any{"notebooks": []any{map[string]any{"name": "NB"}}})))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("summary=%q, want refreshed", res.Summary)
	}
	m, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not a map: %T", res.Data)
	}
	if cached, _ := m["cached"].(bool); cached {
		t.Errorf("first discover should report cached=false")
	}
	if m["filePath"] != "Notebook1.one" || m["fingerprint"] != "fp1" {
		t.Errorf("cache meta wrong: %v", m)
	}
}

func TestDiscover_CacheHit(t *testing.T) {
	store := openTestStore(t)
	// Pre-seed the cache with a matching fingerprint.
	if err := store.Put(doccache.Entry{
		Host:        "onenote",
		FilePath:    "Notebook1.one",
		Fingerprint: "fp1",
		Data:        json.RawMessage(`{"notebooks":[{"name":"NB"}],"filePath":"Notebook1.one","fingerprint":"fp1"}`),
	}); err != nil {
		t.Fatalf("seed put: %v", err)
	}
	srv := cdptest.NewServer(t, okOffice(discoverData("Notebook1.one", "fp1", nil)))
	env := &tools.RunEnv{
		Diag:     &tools.Diagnostics{},
		DocCache: store,
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
	}
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "cache hit") {
		t.Errorf("summary=%q, want cache hit", res.Summary)
	}
	m, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not a map: %T", res.Data)
	}
	if cached, _ := m["cached"].(bool); !cached {
		t.Errorf("cache hit should report cached=true, got %v", m["cached"])
	}
}

func TestDiscover_ForceBypassesCacheHit(t *testing.T) {
	store := openTestStore(t)
	if err := store.Put(doccache.Entry{
		Host:        "onenote",
		FilePath:    "Notebook1.one",
		Fingerprint: "fp1",
		Data:        json.RawMessage(`{"filePath":"Notebook1.one","fingerprint":"fp1"}`),
	}); err != nil {
		t.Fatalf("seed put: %v", err)
	}
	srv := cdptest.NewServer(t, okOffice(discoverData("Notebook1.one", "fp1", nil)))
	env := &tools.RunEnv{
		Diag:     &tools.Diagnostics{},
		DocCache: store,
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
	}
	// force=true must refresh even though the fingerprint matches.
	res := runDiscover(context.Background(), json.RawMessage(`{"force":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("summary=%q, want refreshed", res.Summary)
	}
}

func TestDiscover_OfficeError(t *testing.T) {
	env := fakeEnvWithCache(t, officeErr("AccessDenied", "no permission"))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "AccessDenied" {
		t.Errorf("err=%+v, want office_js/AccessDenied", res.Err)
	}
}

func TestDiscover_AttachFailure(t *testing.T) {
	res := runDiscover(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestDiscover_BadParams(t *testing.T) {
	res := runDiscover(context.Background(),
		json.RawMessage(`{"force":"yes"}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestDiscover_DecodeDiscoverFailure(t *testing.T) {
	// Payload returns a JSON array, not an object => json.Unmarshal into the
	// head struct fails => decode_discover.
	env := fakeEnvWithCache(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice([]any{1, 2, 3}), nil
	})
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "decode_discover" {
		t.Fatalf("want decode_discover, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestDiscover_EmptyFilePath_BypassesCache(t *testing.T) {
	// An empty filePath is not cacheable: Get is a miss and Put is a no-op, so
	// the result is always a fresh refresh with cached=false.
	env := fakeEnvWithCache(t, okOffice(discoverData("", "fp1", nil)))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "discovery refreshed") {
		t.Errorf("summary=%q, want refreshed", res.Summary)
	}
}

func TestDiscover_ToolDefinition(t *testing.T) {
	tool := Discover()
	if tool.Name != "onenote.discover" {
		t.Errorf("name=%q", tool.Name)
	}
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Error("discover should be read-only")
	}
	if tool.Run == nil {
		t.Error("Run is nil")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}
