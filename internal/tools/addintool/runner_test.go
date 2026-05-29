package addintool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/launch"
	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// attachEnv returns a RunEnv whose Attach hands the tool a real *cdp.Connection
// backed by an in-process CDP server driven by resp. Mirrors the proven
// exceltool/template_example_test.go seam.
func attachEnv(t *testing.T, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
	}
}

// connEnv returns a RunEnv whose Conn hands the tool a real *cdp.Connection
// backed by an in-process CDP server. Used by tools that call env.Conn
// (addin.listTargets) rather than env.Attach.
func connEnv(t *testing.T, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Conn: func(context.Context) (*cdp.Connection, error) {
			return srv.Dial(t), nil
		},
	}
}

// attachErrEnv returns a RunEnv whose Attach always fails — exercises the
// attach failure branch without a server.
func attachErrEnv() *tools.RunEnv {
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return nil, errors.New("no target")
		},
	}
}

// targetsResult builds the CDP Target.getTargets result object.
func targetsResult(targets ...cdp.TargetInfo) any {
	return map[string]any{"targetInfos": targets}
}

// -------------------- addin.listTargets --------------------

func TestRunListTargets_HappyPath_NoManifest(t *testing.T) {
	env := connEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.getTargets" {
			return targetsResult(
				cdp.TargetInfo{TargetID: "t1", Type: "page", URL: "https://localhost:3000/taskpane.html"},
				cdp.TargetInfo{TargetID: "t2", Type: "page", URL: "devtools://devtools/bundled/x.html"},
			), nil
		}
		return map[string]any{}, nil
	})
	res := runListTargets(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	// Internal devtools:// target is filtered out by default.
	if !strings.Contains(res.Summary, "Listed 1 CDP target(s)") {
		t.Errorf("summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "(no manifest loaded)") {
		t.Errorf("summary missing no-manifest suffix: %q", res.Summary)
	}
}

func TestRunListTargets_IncludeInternal_WithManifest(t *testing.T) {
	env := connEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.getTargets" {
			return targetsResult(
				cdp.TargetInfo{TargetID: "t1", Type: "page", URL: "https://localhost:3000/taskpane.html"},
				cdp.TargetInfo{TargetID: "t2", Type: "page", URL: "edge://settings"},
			), nil
		}
		return map[string]any{}, nil
	})
	env.Manifest = func() *addin.Manifest {
		return &addin.Manifest{ID: "abc", DisplayName: "My Add-in"}
	}
	res := runListTargets(context.Background(), json.RawMessage(`{"includeInternal":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	// Both targets visible because includeInternal=true; manifest suffix absent.
	if !strings.Contains(res.Summary, "Listed 2 CDP target(s).") {
		t.Errorf("summary=%q", res.Summary)
	}
	out, ok := res.Data.(struct {
		Targets     []addin.ClassifiedTarget `json:"targets"`
		Manifest    *addin.Manifest          `json:"manifest,omitempty"`
		HasManifest bool                     `json:"hasManifest"`
	})
	if !ok {
		t.Fatalf("Data type %T unexpected", res.Data)
	}
	if !out.HasManifest || out.Manifest == nil || out.Manifest.ID != "abc" {
		t.Errorf("manifest not propagated: %+v", out)
	}
}

func TestRunListTargets_BadParams(t *testing.T) {
	res := runListTargets(context.Background(), json.RawMessage(`{"includeInternal":"nope"}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category=%q, want validation", res.Err.Category)
	}
}

func TestRunListTargets_ConnFailure(t *testing.T) {
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Conn: func(context.Context) (*cdp.Connection, error) {
			return nil, errors.New("dial failed")
		},
	}
	res := runListTargets(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "open_failed" {
		t.Fatalf("want open_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryConnection || !res.Err.Retryable {
		t.Errorf("err=%+v, want connection/retryable", res.Err)
	}
}

func TestRunListTargets_GetTargetsFailure(t *testing.T) {
	env := connEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.getTargets" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "target enumeration failed"}
		}
		return map[string]any{}, nil
	})
	res := runListTargets(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "get_targets_failed" {
		t.Fatalf("want get_targets_failed, got %+v", res.Err)
	}
	// ClassifyCDPErr surfaces the structured cdpError in Details.
	if res.Err.Details == nil || res.Err.Details["cdpError"] == nil {
		t.Errorf("expected cdpError details, got %+v", res.Err.Details)
	}
}

// -------------------- addin.contextInfo --------------------

func TestRunContextInfo_HappyPath(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"context": map[string]any{"host": "Excel", "platform": "PC"},
			}), nil
		}
		return map[string]any{}, nil
	})
	res := runContextInfo(context.Background(), json.RawMessage(`{"surface":"taskpane"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Returned Office context (host=Excel)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunContextInfo_HappyPath_NoHost(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"platform": "PC"}), nil
		}
		return map[string]any{}, nil
	})
	res := runContextInfo(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Returned Office context info." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunContextInfo_HostAtTopLevel(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"host": "Word"}), nil
		}
		return map[string]any{}, nil
	})
	res := runContextInfo(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Returned Office context (host=Word)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunContextInfo_WithManifestRequirements(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"context": map[string]any{"host": "Excel"}}), nil
		}
		return map[string]any{}, nil
	})
	env.Manifest = func() *addin.Manifest {
		return &addin.Manifest{
			ID:           "abc",
			Requirements: []addin.RequirementSet{{Name: "CustomSet", MinVersion: "9.9"}},
		}
	}
	res := runContextInfo(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

func TestRunContextInfo_CustomRequirementSets(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"context": map[string]any{"host": "Excel"}}), nil
		}
		return map[string]any{}, nil
	})
	res := runContextInfo(context.Background(),
		json.RawMessage(`{"requirementSets":[{"name":"ExcelApi","minVersion":"1.1"}]}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

func TestRunContextInfo_OfficeError(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOfficeErr("office_unavailable", "Office.js not loaded", map[string]any{"x": 1}), nil
		}
		return map[string]any{}, nil
	})
	res := runContextInfo(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "office_unavailable" {
		t.Errorf("err=%+v, want office_js/office_unavailable", res.Err)
	}
	if res.Err.RecoveryHint == "" {
		t.Error("expected non-empty recoveryHint for known office code")
	}
	if res.Err.Details == nil || res.Err.Details["debugInfo"] == nil {
		t.Errorf("expected debugInfo details, got %+v", res.Err.Details)
	}
}

func TestRunContextInfo_AttachFailure(t *testing.T) {
	res := runContextInfo(context.Background(), json.RawMessage(`{}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category=%q, want not_found", res.Err.Category)
	}
}

func TestRunContextInfo_BadParams(t *testing.T) {
	res := runContextInfo(context.Background(), json.RawMessage(`{"targetId":123}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestContextInfoHost(t *testing.T) {
	if h, ok := contextInfoHost("not a map"); ok || h != "" {
		t.Errorf("non-map: got (%q,%v)", h, ok)
	}
	if h, ok := contextInfoHost(map[string]any{"context": map[string]any{"host": "Excel"}}); !ok || h != "Excel" {
		t.Errorf("nested: got (%q,%v)", h, ok)
	}
	if h, ok := contextInfoHost(map[string]any{"host": "Outlook"}); !ok || h != "Outlook" {
		t.Errorf("top-level: got (%q,%v)", h, ok)
	}
	if h, ok := contextInfoHost(map[string]any{"other": 1}); ok || h != "" {
		t.Errorf("missing: got (%q,%v)", h, ok)
	}
	// context present but host wrong type / absent → falls through, no top-level host.
	if h, ok := contextInfoHost(map[string]any{"context": map[string]any{"host": 42}}); ok || h != "" {
		t.Errorf("wrong-type host: got (%q,%v)", h, ok)
	}
}

// -------------------- addin.cfRuntimeInfo --------------------

func TestRunCFRuntimeInfo_NotAvailable(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"available": false}), nil
		}
		return map[string]any{}, nil
	})
	res := runCFRuntimeInfo(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Custom-functions runtime not exposed in target." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunCFRuntimeInfo_WithFunctions(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"available": true,
				"functions": []any{"ADD", "SUB"},
			}), nil
		}
		return map[string]any{}, nil
	})
	res := runCFRuntimeInfo(context.Background(), json.RawMessage(`{"targetId":"t9"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Found 2 registered custom function(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunCFRuntimeInfo_WithMappings(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"available": true,
				"mappings":  map[string]any{"ADD": "add"},
			}), nil
		}
		return map[string]any{}, nil
	})
	res := runCFRuntimeInfo(context.Background(), json.RawMessage(`{"urlPattern":"functions"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Found 1 custom-function mapping(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunCFRuntimeInfo_AvailableButEmpty(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"available": true}), nil
		}
		return map[string]any{}, nil
	})
	res := runCFRuntimeInfo(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Custom-functions runtime exposed but no functions registered." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunCFRuntimeInfo_OfficeError(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOfficeErr("requirement_unmet", "no cf runtime", nil), nil
		}
		return map[string]any{}, nil
	})
	res := runCFRuntimeInfo(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "requirement_unmet" {
		t.Fatalf("want requirement_unmet, got %+v", res.Err)
	}
	if res.Err.RecoveryHint == "" {
		t.Error("expected recoveryHint for requirement_unmet")
	}
}

func TestRunCFRuntimeInfo_AttachFailure(t *testing.T) {
	res := runCFRuntimeInfo(context.Background(), json.RawMessage(`{}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunCFRuntimeInfo_BadParams(t *testing.T) {
	res := runCFRuntimeInfo(context.Background(), json.RawMessage(`{"targetId":5}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// -------------------- addin.openDialog --------------------

func TestRunOpenDialog_HappyPath(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"opened": true}), nil
		}
		return map[string]any{}, nil
	})
	res := runOpenDialog(context.Background(),
		json.RawMessage(`{"url":"https://localhost:3000/dialog.html","height":50,"width":40,"displayInIframe":true,"promptBeforeOpen":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Opened dialog at https://localhost:3000/dialog.html." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunOpenDialog_DefaultsToTaskpaneSurface(t *testing.T) {
	var gotSel tools.TargetSelector
	srv := cdptest.NewServer(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"opened": true}), nil
		}
		return map[string]any{}, nil
	})
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(_ context.Context, sel tools.TargetSelector) (*tools.AttachedTarget, error) {
			gotSel = sel
			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
	}
	res := runOpenDialog(context.Background(), json.RawMessage(`{"url":"https://x/d.html"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if gotSel.Surface != addin.SurfaceTaskpane {
		t.Errorf("default surface=%q, want taskpane", gotSel.Surface)
	}
}

func TestRunOpenDialog_OfficeError(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOfficeErr("DialogAlreadyOpen", "a dialog is already open", nil), nil
		}
		return map[string]any{}, nil
	})
	res := runOpenDialog(context.Background(), json.RawMessage(`{"url":"https://x/d.html"}`), env)
	if res.Err == nil || res.Err.Code != "DialogAlreadyOpen" {
		t.Fatalf("want DialogAlreadyOpen, got %+v", res.Err)
	}
	// Unknown office code → no recoveryHint.
	if res.Err.RecoveryHint != "" {
		t.Errorf("unexpected recoveryHint for unknown code: %q", res.Err.RecoveryHint)
	}
}

func TestRunOpenDialog_AttachFailure(t *testing.T) {
	res := runOpenDialog(context.Background(), json.RawMessage(`{"url":"https://x/d.html"}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunOpenDialog_BadParams(t *testing.T) {
	res := runOpenDialog(context.Background(), json.RawMessage(`{"url":123}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// -------------------- addin.dialogClose / dialogSubscribe --------------------

func TestRunDialogClose_Closed(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"closed": true}), nil
		}
		return map[string]any{}, nil
	})
	res := DialogClose().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Closed active dialog." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunDialogClose_NoHandle(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"closed": false}), nil
		}
		return map[string]any{}, nil
	})
	res := DialogClose().Run(context.Background(), json.RawMessage(`{"surface":"dialog"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "No active dialog handle to close." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunDialogSubscribe_Drained(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"messages": []any{"m1", "m2", "m3"},
				"events":   []any{"e1"},
			}), nil
		}
		return map[string]any{}, nil
	})
	res := DialogSubscribe().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Drained 3 message(s) and 1 event(s) from dialog." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunDialogPayload_OfficeError(t *testing.T) {
	env := attachEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOfficeErr("office_ready_timeout", "not ready", nil), nil
		}
		return map[string]any{}, nil
	})
	res := DialogSubscribe().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "office_ready_timeout" {
		t.Fatalf("want office_ready_timeout, got %+v", res.Err)
	}
	if res.Err.RecoveryHint == "" {
		t.Error("expected recoveryHint for office_ready_timeout")
	}
}

func TestRunDialogPayload_AttachFailure(t *testing.T) {
	res := DialogClose().Run(context.Background(), json.RawMessage(`{}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunDialogPayload_BadParams(t *testing.T) {
	res := DialogClose().Run(context.Background(), json.RawMessage(`{"surface":1}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunDialogPayload_DefaultsToTaskpaneSurface(t *testing.T) {
	var gotSel tools.TargetSelector
	srv := cdptest.NewServer(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"closed": true}), nil
		}
		return map[string]any{}, nil
	})
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(_ context.Context, sel tools.TargetSelector) (*tools.AttachedTarget, error) {
			gotSel = sel
			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
	}
	res := DialogClose().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if gotSel.Surface != addin.SurfaceTaskpane {
		t.Errorf("default surface=%q, want taskpane", gotSel.Surface)
	}
}

func TestDialogPayloadSummary(t *testing.T) {
	if s := dialogPayloadSummary("addin.dialogClose", json.RawMessage(`{"closed":true}`)); s != "Closed active dialog." {
		t.Errorf("close-true=%q", s)
	}
	if s := dialogPayloadSummary("addin.dialogClose", json.RawMessage(`not-json`)); s != "No active dialog handle to close." {
		t.Errorf("close-badjson=%q", s)
	}
	if s := dialogPayloadSummary("addin.dialogSubscribe", json.RawMessage(`{"messages":[1],"events":[2,3]}`)); s != "Drained 1 message(s) and 2 event(s) from dialog." {
		t.Errorf("subscribe=%q", s)
	}
	if s := dialogPayloadSummary("addin.dialogSubscribe", json.RawMessage(`not-json`)); s != "Drained dialog messages." {
		t.Errorf("subscribe-badjson=%q", s)
	}
	if s := dialogPayloadSummary("addin.unknown", json.RawMessage(`{}`)); s != "" {
		t.Errorf("unknown=%q", s)
	}
}

// -------------------- decodePayloadResultWithSummary --------------------

func TestDecodePayloadResultWithSummary(t *testing.T) {
	res := decodePayloadResultWithSummary(json.RawMessage(`{"ok":1}`), "hi")
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "hi" {
		t.Errorf("summary=%q", res.Summary)
	}
	bad := decodePayloadResultWithSummary(json.RawMessage(`{bad`), "hi")
	if bad.Err == nil || bad.Err.Code != "decode_payload_result" {
		t.Fatalf("want decode_payload_result, got %+v", bad.Err)
	}
	if bad.Err.Category != tools.CategoryInternal {
		t.Errorf("category=%q, want internal", bad.Err.Category)
	}
}

// -------------------- errors.go: mapPayloadError --------------------

func TestMapPayloadError_OfficeError_WithDebugInfo(t *testing.T) {
	oerr := &officejs.OfficeError{
		Code:      "ItemNotFound",
		Message:   "nope",
		DebugInfo: json.RawMessage(`{"errorLocation":"x"}`),
	}
	res := mapPayloadError(oerr)
	if res.Err == nil || res.Err.Code != "ItemNotFound" || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("err=%+v", res.Err)
	}
	if res.Err.Details["debugInfo"] == nil {
		t.Errorf("debugInfo missing: %+v", res.Err.Details)
	}
}

func TestMapPayloadError_OfficeError_EmptyCode(t *testing.T) {
	res := mapPayloadError(&officejs.OfficeError{Message: "boom"})
	if res.Err == nil || res.Err.Code != "office_js_error" {
		t.Fatalf("want office_js_error fallback, got %+v", res.Err)
	}
}

func TestMapPayloadError_OfficeError_BadDebugInfo(t *testing.T) {
	// DebugInfo present but not valid JSON → not added to details.
	res := mapPayloadError(&officejs.OfficeError{
		Code:      "X",
		Message:   "m",
		DebugInfo: json.RawMessage(`{not json`),
	})
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if _, ok := res.Err.Details["debugInfo"]; ok {
		t.Errorf("debugInfo should be absent for invalid JSON: %+v", res.Err.Details)
	}
}

func TestMapPayloadError_ProtocolException(t *testing.T) {
	res := mapPayloadError(&officejs.ProtocolException{Text: "SyntaxError"})
	if res.Err == nil || res.Err.Code != "payload_protocol_exception" {
		t.Fatalf("want payload_protocol_exception, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryProtocol {
		t.Errorf("category=%q, want protocol", res.Err.Category)
	}
}

func TestMapPayloadError_GenericCDPError(t *testing.T) {
	res := mapPayloadError(errors.New("ws closed unexpectedly"))
	if res.Err == nil || res.Err.Code != "payload_failed" {
		t.Fatalf("want payload_failed, got %+v", res.Err)
	}
}

func TestRecoveryHintForOfficeCode(t *testing.T) {
	for _, code := range []string{
		"office_unavailable",
		"office_ready_failed",
		"office_ready_timeout",
		"requirement_unmet",
		"requirement_check_failed",
	} {
		if recoveryHintForOfficeCode(code) == "" {
			t.Errorf("code %q: expected non-empty hint", code)
		}
	}
	if recoveryHintForOfficeCode("totally_unknown") != "" {
		t.Error("unknown code should map to empty hint")
	}
}

// -------------------- ensurerunning.go helpers --------------------

func TestDetectErrMessage(t *testing.T) {
	if got := detectErrMessage(nil, "C:\\proj"); got != "no add-in project resolved from C:\\proj" {
		t.Errorf("nil err=%q", got)
	}
	if got := detectErrMessage(errors.New("boom"), "C:\\proj"); got != "boom" {
		t.Errorf("with err=%q", got)
	}
}

func TestCodeFromReason(t *testing.T) {
	if got := codeFromReason(""); got != "launch_failed" {
		t.Errorf("empty=%q", got)
	}
	if got := codeFromReason(launch.ReasonCDPNotReady); got != "launch_cdp-not-ready" {
		t.Errorf("cdp=%q", got)
	}
}

func TestLaunchErrToResult_NotALaunchError(t *testing.T) {
	res := launchErrToResult(errors.New("plain error"))
	if res.Err == nil || res.Err.Code != "launch_failed" || res.Err.Category != tools.CategoryInternal {
		t.Fatalf("err=%+v", res.Err)
	}
	if !strings.Contains(res.Summary, "plain error") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestLaunchErrToResult_ReasonMapping(t *testing.T) {
	cases := []struct {
		reason       string
		wantCode     string
		wantCategory string
		wantRetry    bool
		wantHint     bool
	}{
		{launch.ReasonUnsupportedPlatform, "launch_unsupported-platform", tools.CategoryUnsupported, false, true},
		{launch.ReasonLauncherMissing, "launch_launcher-missing", tools.CategoryUnsupported, false, true},
		{launch.ReasonPortAlreadyConfig, "launch_port-already-configured", tools.CategoryUnsupported, false, true},
		{launch.ReasonCDPNotReady, "launch_cdp-not-ready", tools.CategoryTimeout, true, true},
		{launch.ReasonDevServerNotReady, "launch_dev-server-not-ready", tools.CategoryTimeout, true, true},
		{launch.ReasonLaunchFailed, "launch_launch-failed", tools.CategoryInternal, false, false},
	}
	for _, c := range cases {
		t.Run(c.reason, func(t *testing.T) {
			le := &launch.LaunchError{Reason: c.reason, Message: "msg", Output: []string{"line1"}}
			res := launchErrToResult(le)
			if res.Err == nil {
				t.Fatalf("expected error for %s", c.reason)
			}
			if res.Err.Code != c.wantCode {
				t.Errorf("code=%q, want %q", res.Err.Code, c.wantCode)
			}
			if res.Err.Category != c.wantCategory {
				t.Errorf("category=%q, want %q", res.Err.Category, c.wantCategory)
			}
			if res.Err.Retryable != c.wantRetry {
				t.Errorf("retryable=%v, want %v", res.Err.Retryable, c.wantRetry)
			}
			if (res.Err.RecoveryHint != "") != c.wantHint {
				t.Errorf("hint=%q, wantHint=%v", res.Err.RecoveryHint, c.wantHint)
			}
			if res.Err.Details["reason"] != c.reason {
				t.Errorf("details.reason=%v, want %q", res.Err.Details["reason"], c.reason)
			}
			if res.Err.Details["output"] == nil {
				t.Errorf("expected output in details: %+v", res.Err.Details)
			}
		})
	}
}

// -------------------- addin.ensureRunning (run path) --------------------

func TestRunEnsureRunning_BadParams(t *testing.T) {
	res := runEnsureRunning(context.Background(), json.RawMessage(`{"port":"nope"}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunEnsureRunning_NoProjectNoExcel(t *testing.T) {
	// cwd is an empty temp dir — no add-in manifest in any ancestor — and port 1
	// has nothing listening, so DetectAddin fails AND the probe misses → the
	// friendly addin_not_found branch.
	env := &tools.RunEnv{Diag: &tools.Diagnostics{}}
	res := runEnsureRunning(context.Background(),
		json.RawMessage(fmt.Sprintf(`{"cwd":%q,"port":1,"timeoutMs":1000}`, t.TempDir())), env)
	if res.Err == nil || res.Err.Code != "addin_not_found" {
		t.Fatalf("want addin_not_found, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category=%q, want not_found", res.Err.Category)
	}
	if res.Err.Details["recoverableViaTool"] != "addin.detect" {
		t.Errorf("recoverableViaTool=%v", res.Err.Details["recoverableViaTool"])
	}
}

func TestRunEnsureRunning_Preexisting(t *testing.T) {
	// Stand up a fake CDP /json/version responder bound to a 127.0.0.1 port so
	// LaunchIfNeeded's probe (it dials http://localhost:<port>) short-circuits
	// to "preexisting" without any spawn.
	port := serveCDPVersion(t)

	var endpointSet bool
	env := &tools.RunEnv{
		Diag:        &tools.Diagnostics{},
		SetEndpoint: func(webview2.Config) { endpointSet = true },
	}
	res := runEnsureRunning(context.Background(),
		json.RawMessage(fmt.Sprintf(`{"cwd":%q,"port":%d,"timeoutMs":2000}`, t.TempDir(), port)), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data type %T, want map", res.Data)
	}
	if out["source"] != "preexisting" {
		t.Errorf("source=%v, want preexisting", out["source"])
	}
	if !strings.Contains(res.Summary, "already reachable") {
		t.Errorf("summary=%q", res.Summary)
	}
	if !endpointSet {
		t.Error("SetEndpoint was not invoked")
	}
}

func TestRunEnsureRunning_Preexisting_ResetSessionsNotCalled(t *testing.T) {
	// A "preexisting" hit must NOT call ResetSessions (only "launched" does).
	port := serveCDPVersion(t)

	resetCalled := false
	env := &tools.RunEnv{
		Diag:          &tools.Diagnostics{},
		ResetSessions: func() { resetCalled = true },
	}
	res := runEnsureRunning(context.Background(),
		json.RawMessage(fmt.Sprintf(`{"cwd":%q,"port":%d,"timeoutMs":2000}`, t.TempDir(), port)), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if resetCalled {
		t.Error("ResetSessions called on preexisting hit; want only on launched")
	}
}

func TestRunEnsureRunning_Preexisting_DefaultCWD(t *testing.T) {
	// Omitting cwd exercises the os.Getwd() branch. The probe still
	// short-circuits to "preexisting" so no detection/launch is attempted, and
	// the nil SetManifest/SetEndpoint/ResetSessions hooks confirm nil-safety.
	port := serveCDPVersion(t)
	env := &tools.RunEnv{Diag: &tools.Diagnostics{}}
	res := runEnsureRunning(context.Background(),
		json.RawMessage(fmt.Sprintf(`{"port":%d,"timeoutMs":2000}`, port)), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data type %T, want map", res.Data)
	}
	if out["source"] != "preexisting" {
		t.Errorf("source=%v, want preexisting", out["source"])
	}
}

// serveCDPVersion binds an HTTP server to a fresh 127.0.0.1 port that replies
// to /json/version with a CDP browser-version document, and returns the port.
// LaunchIfNeeded / ProbeCDPEndpoint dial http://localhost:<port>, which on the
// loopback resolves to 127.0.0.1.
func serveCDPVersion(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Browser":"WebView2/Test"}`))
	})
	httpSrv := &http.Server{Handler: mux} //nolint:gosec // test server; no timeouts needed.
	go func() { _ = httpSrv.Serve(ln) }()
	t.Cleanup(func() { _ = httpSrv.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

// cdpVersionWSResponder builds a combined endpoint for addin.status's happy
// path: an HTTP /json/version document whose webSocketDebuggerUrl points at the
// in-process cdptest CDP websocket server. Returns the BrowserURL to feed into
// env.Endpoint.
func cdpVersionWSResponder(t *testing.T, resp cdptest.Responder) string {
	t.Helper()
	cdpSrv := cdptest.NewServer(t, resp)
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/json/version") {
			http.NotFound(w, r)
			return
		}
		body := map[string]string{"Browser": "WebView2/Test", "webSocketDebuggerUrl": cdpSrv.WSURL()}
		raw, err := json.Marshal(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(raw)
	}))
	t.Cleanup(httpSrv.Close)
	return httpSrv.URL
}

// -------------------- addin.status (reachable paths) --------------------

func TestRunStatus_BadParams(t *testing.T) {
	res := runStatus(context.Background(), json.RawMessage(`{"includeInternal":"nope"}`), &tools.RunEnv{})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunStatus_Reachable_TargetsVisible(t *testing.T) {
	browserURL := cdpVersionWSResponder(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.getTargets" {
			return targetsResult(
				cdp.TargetInfo{TargetID: "t1", Type: "page", URL: "https://localhost:3000/taskpane.html"},
				cdp.TargetInfo{TargetID: "t2", Type: "page", URL: "edge://settings"},
			), nil
		}
		return map[string]any{}, nil
	})
	env := &tools.RunEnv{Endpoint: webview2.Config{BrowserURL: browserURL}}
	res := runStatus(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out, ok := res.Data.(statusOutput)
	if !ok {
		t.Fatalf("Data type %T, want statusOutput", res.Data)
	}
	if !out.Endpoint.Reachable {
		t.Error("Endpoint.Reachable = false, want true")
	}
	// edge:// internal target filtered → only the taskpane is visible.
	if len(out.Targets) != 1 || out.Targets[0].TargetID != "t1" {
		t.Errorf("targets=%+v, want only t1", out.Targets)
	}
	// No manifest loaded → recovery hint mentioning addin.detect/launch.
	if len(out.RecoveryHints) == 0 {
		t.Error("expected a no-manifest recovery hint")
	}
	if !strings.Contains(res.Summary, "1 target(s) visible") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunStatus_Reachable_WithManifest_IncludeInternal(t *testing.T) {
	browserURL := cdpVersionWSResponder(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.getTargets" {
			return targetsResult(
				cdp.TargetInfo{TargetID: "t1", Type: "page", URL: "https://localhost:3000/taskpane.html"},
				cdp.TargetInfo{TargetID: "t2", Type: "page", URL: "edge://settings"},
			), nil
		}
		return map[string]any{}, nil
	})
	env := &tools.RunEnv{
		Endpoint: webview2.Config{BrowserURL: browserURL},
		Manifest: func() *addin.Manifest {
			return &addin.Manifest{ID: "abc", DisplayName: "My Add-in", Path: "C:\\m.xml", Hosts: []string{"Workbook"}}
		},
	}
	res := runStatus(context.Background(), json.RawMessage(`{"includeInternal":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(statusOutput)
	if len(out.Targets) != 2 {
		t.Errorf("targets=%d, want 2 (includeInternal)", len(out.Targets))
	}
	if !out.Manifest.Loaded || out.Manifest.DisplayName != "My Add-in" {
		t.Errorf("manifest=%+v, want loaded with DisplayName", out.Manifest)
	}
	if !strings.Contains(res.Summary, "My Add-in") {
		t.Errorf("summary=%q, want manifest display name", res.Summary)
	}
}

func TestRunStatus_Reachable_NoVisibleTargets(t *testing.T) {
	browserURL := cdpVersionWSResponder(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.getTargets" {
			return targetsResult(
				cdp.TargetInfo{TargetID: "t2", Type: "page", URL: "edge://settings"},
			), nil
		}
		return map[string]any{}, nil
	})
	env := &tools.RunEnv{Endpoint: webview2.Config{BrowserURL: browserURL}}
	res := runStatus(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(statusOutput)
	if len(out.Targets) != 0 {
		t.Errorf("targets=%d, want 0", len(out.Targets))
	}
	// "No add-in targets visible" + no-manifest → at least two hints.
	if len(out.RecoveryHints) < 2 {
		t.Errorf("recoveryHints=%v, want >=2", out.RecoveryHints)
	}
}

func TestRunStatus_Reachable_GetTargetsFailure(t *testing.T) {
	browserURL := cdpVersionWSResponder(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.getTargets" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "enum failed"}
		}
		return map[string]any{}, nil
	})
	env := &tools.RunEnv{Endpoint: webview2.Config{BrowserURL: browserURL}}
	res := runStatus(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("expected OK envelope (failures encoded in payload), got %+v", res.Err)
	}
	out := res.Data.(statusOutput)
	if !out.Endpoint.Reachable {
		t.Error("Endpoint.Reachable should be true even when getTargets fails")
	}
	if !strings.Contains(res.Summary, "Target.getTargets failed") {
		t.Errorf("summary=%q", res.Summary)
	}
	if len(out.RecoveryHints) == 0 {
		t.Error("expected a getTargets-failed recovery hint")
	}
}

func TestRunStatus_Reachable_WSDialFailure(t *testing.T) {
	// /json/version resolves to a webSocketDebuggerUrl that nothing serves, so
	// Discover succeeds but cdp.Dial fails.
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/json/version") {
			http.NotFound(w, r)
			return
		}
		body := map[string]string{"Browser": "WebView2/Test", "webSocketDebuggerUrl": "ws://127.0.0.1:1/devtools/browser/dead"}
		raw, err := json.Marshal(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(raw)
	}))
	t.Cleanup(httpSrv.Close)

	env := &tools.RunEnv{Endpoint: webview2.Config{BrowserURL: httpSrv.URL}}
	res := runStatus(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("expected OK envelope, got %+v", res.Err)
	}
	out := res.Data.(statusOutput)
	if !out.Endpoint.Reachable {
		t.Error("Endpoint.Reachable should be true (discovery succeeded)")
	}
	if !strings.Contains(res.Summary, "WebSocket dial failed") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestManifestSummary(t *testing.T) {
	if got := manifestSummary(nil); got.Loaded {
		t.Errorf("nil env: Loaded=true, want false")
	}
	if got := manifestSummary(&tools.RunEnv{}); got.Loaded {
		t.Errorf("nil Manifest: Loaded=true, want false")
	}
	if got := manifestSummary(&tools.RunEnv{Manifest: func() *addin.Manifest { return nil }}); got.Loaded {
		t.Errorf("Manifest()=nil: Loaded=true, want false")
	}
	env := &tools.RunEnv{Manifest: func() *addin.Manifest {
		return &addin.Manifest{ID: "id1", DisplayName: "Name", Path: "p", Hosts: []string{"Workbook"}}
	}}
	got := manifestSummary(env)
	if !got.Loaded || got.ID != "id1" || got.DisplayName != "Name" || got.Path != "p" || len(got.Hosts) != 1 {
		t.Errorf("summary=%+v", got)
	}
}

func TestRunStatus_Reachable_ManifestEmptyDisplayName(t *testing.T) {
	// Manifest loaded but DisplayName empty → summary label falls back to ID.
	browserURL := cdpVersionWSResponder(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.getTargets" {
			return targetsResult(
				cdp.TargetInfo{TargetID: "t1", Type: "page", URL: "https://localhost:3000/taskpane.html"},
			), nil
		}
		return map[string]any{}, nil
	})
	env := &tools.RunEnv{
		Endpoint: webview2.Config{BrowserURL: browserURL},
		Manifest: func() *addin.Manifest { return &addin.Manifest{ID: "only-id"} },
	}
	res := runStatus(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "only-id") {
		t.Errorf("summary=%q, want fallback to ID", res.Summary)
	}
}
