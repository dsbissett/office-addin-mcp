package pagetool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// connEnv returns a RunEnv whose Conn hands the tool a real *cdp.Connection
// backed by an in-process CDP server driven by resp. Used by tools that take
// the env.Conn path (pages.list, pages.close).
func connEnv(t *testing.T, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	conn := srv.Dial(t)
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Conn: func(context.Context) (*cdp.Connection, error) { return conn, nil },
	}
}

// attachEnv returns a RunEnv whose Attach hands a resolved target plus a
// connection backed by srv. Used by tools that take the env.Attach path
// (pages.select, pages.close, page.navigate, pages.handleDialog).
func attachEnv(t *testing.T, target cdp.TargetInfo, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: srv.Dial(t), Target: target, SessionID: "cdp-1"}, nil
		},
	}
}

// attachErrEnv returns a RunEnv whose Attach always fails.
func attachErrEnv() *tools.RunEnv {
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return nil, errors.New("no target")
		},
	}
}

// ---------------------------------------------------------------------------
// pages.list
// ---------------------------------------------------------------------------

func TestRunList_HappyPath_FiltersNonPageAndInternal(t *testing.T) {
	env := connEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method != "Target.getTargets" {
			t.Errorf("unexpected method %q", method)
		}
		return map[string]any{"targetInfos": []map[string]any{
			{"targetId": "T1", "type": "page", "url": "https://localhost:3000/taskpane.html"},
			{"targetId": "T2", "type": "service_worker", "url": "https://localhost:3000/sw.js"},
			{"targetId": "T3", "type": "page", "url": "devtools://devtools/bundled/x.html"},
		}}, nil
	})
	res := runList(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Listed 1 page target(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
	data, ok := res.Data.(struct {
		Pages       []addin.ClassifiedTarget `json:"pages"`
		HasManifest bool                     `json:"hasManifest"`
	})
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if data.HasManifest {
		t.Errorf("hasManifest should be false without a manifest")
	}
	if len(data.Pages) != 1 || data.Pages[0].TargetID != "T1" {
		t.Fatalf("pages=%+v, want only T1", data.Pages)
	}
}

func TestRunList_IncludeInternal(t *testing.T) {
	env := connEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return map[string]any{"targetInfos": []map[string]any{
			{"targetId": "T1", "type": "page", "url": "https://localhost:3000/taskpane.html"},
			{"targetId": "T3", "type": "page", "url": "devtools://devtools/bundled/x.html"},
		}}, nil
	})
	res := runList(context.Background(), json.RawMessage(`{"includeInternal":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	data := res.Data.(struct {
		Pages       []addin.ClassifiedTarget `json:"pages"`
		HasManifest bool                     `json:"hasManifest"`
	})
	if len(data.Pages) != 2 {
		t.Fatalf("want 2 pages with includeInternal, got %+v", data.Pages)
	}
}

func TestRunList_WithManifest(t *testing.T) {
	env := connEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return map[string]any{"targetInfos": []map[string]any{
			{"targetId": "T1", "type": "page", "url": "https://localhost:3000/taskpane.html"},
		}}, nil
	})
	env.Manifest = func() *addin.Manifest {
		return &addin.Manifest{
			ID:          "abc",
			DisplayName: "My Add-in",
			Surfaces:    []addin.Surface{{Type: addin.SurfaceTaskpane, URL: "https://localhost:3000/taskpane.html", Pattern: "taskpane.html"}},
		}
	}
	res := runList(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	data := res.Data.(struct {
		Pages       []addin.ClassifiedTarget `json:"pages"`
		HasManifest bool                     `json:"hasManifest"`
	})
	if !data.HasManifest {
		t.Errorf("hasManifest should be true")
	}
	if len(data.Pages) != 1 || data.Pages[0].Surface != addin.SurfaceTaskpane {
		t.Fatalf("expected taskpane-classified target, got %+v", data.Pages)
	}
}

func TestRunList_ManifestReturnsNil(t *testing.T) {
	env := connEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return map[string]any{"targetInfos": []map[string]any{
			{"targetId": "T1", "type": "page", "url": "https://localhost:3000/taskpane.html"},
		}}, nil
	})
	env.Manifest = func() *addin.Manifest { return nil }
	res := runList(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	data := res.Data.(struct {
		Pages       []addin.ClassifiedTarget `json:"pages"`
		HasManifest bool                     `json:"hasManifest"`
	})
	if data.HasManifest {
		t.Errorf("hasManifest should be false when Manifest() returns nil")
	}
}

func TestRunList_BadParams(t *testing.T) {
	res := runList(context.Background(), json.RawMessage(`{"includeInternal":"nope"}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunList_ConnFailure(t *testing.T) {
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Conn: func(context.Context) (*cdp.Connection, error) { return nil, errors.New("dial refused") },
	}
	res := runList(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "open_failed" {
		t.Fatalf("want open_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryConnection || !res.Err.Retryable {
		t.Errorf("err=%+v, want connection/retryable", res.Err)
	}
}

func TestRunList_GetTargetsFailure(t *testing.T) {
	env := connEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return nil, &cdp.RemoteError{Code: -32000, Message: "boom"}
	})
	res := runList(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "get_targets_failed" {
		t.Fatalf("want get_targets_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryProtocol {
		t.Errorf("category=%q", res.Err.Category)
	}
}

// ---------------------------------------------------------------------------
// pages.select
// ---------------------------------------------------------------------------

func TestRunSelect_HappyPath_UsesTitle(t *testing.T) {
	var gotTarget cdp.TargetInfo
	var gotSID string
	env := attachEnv(t, cdp.TargetInfo{TargetID: "T1", URL: "https://localhost/taskpane.html", Title: "Task Pane"}, nil)
	env.SetDefaultSelection = func(target cdp.TargetInfo, cdpSessionID string) {
		gotTarget = target
		gotSID = cdpSessionID
	}
	res := runSelect(context.Background(), json.RawMessage(`{"targetId":"T1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Selected page Task Pane." {
		t.Errorf("summary=%q", res.Summary)
	}
	if gotTarget.TargetID != "T1" || gotSID != "cdp-1" {
		t.Errorf("SetDefaultSelection got target=%+v sid=%q", gotTarget, gotSID)
	}
	data := res.Data.(struct {
		TargetID     string `json:"targetId"`
		URL          string `json:"url"`
		Title        string `json:"title,omitempty"`
		CDPSessionID string `json:"cdpSessionId"`
	})
	if data.TargetID != "T1" || data.CDPSessionID != "cdp-1" || data.Title != "Task Pane" {
		t.Errorf("data=%+v", data)
	}
}

func TestRunSelect_HappyPath_FallsBackToURLLabel(t *testing.T) {
	env := attachEnv(t, cdp.TargetInfo{TargetID: "T2", URL: "https://localhost/x.html"}, nil)
	// SetDefaultSelection deliberately left nil to exercise the nil-guard.
	res := runSelect(context.Background(), json.RawMessage(`{"urlPattern":"x.html"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Selected page https://localhost/x.html." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunSelect_MissingSelector(t *testing.T) {
	res := runSelect(context.Background(), json.RawMessage(`{}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil || res.Err.Code != "missing_selector" {
		t.Fatalf("want missing_selector, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunSelect_BadParams(t *testing.T) {
	res := runSelect(context.Background(), json.RawMessage(`{"targetId":123}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunSelect_AttachFailure(t *testing.T) {
	res := runSelect(context.Background(), json.RawMessage(`{"targetId":"T1"}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category=%q", res.Err.Category)
	}
}

// ---------------------------------------------------------------------------
// pages.close
// ---------------------------------------------------------------------------

// closeEnv wires both Conn and Attach against one in-process server so close's
// resolve-then-close path is exercised end to end.
func closeEnv(t *testing.T, target cdp.TargetInfo, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	conn := srv.Dial(t)
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Conn: func(context.Context) (*cdp.Connection, error) { return conn, nil },
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: conn, Target: target, SessionID: "cdp-1"}, nil
		},
	}
}

func TestRunClose_HappyPath(t *testing.T) {
	cleared := false
	env := closeEnv(t, cdp.TargetInfo{TargetID: "T1"}, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.closeTarget" {
			return map[string]any{"success": true}, nil
		}
		return map[string]any{}, nil
	})
	env.ClearDefaultSelection = func() { cleared = true }
	res := runClose(context.Background(), json.RawMessage(`{"targetId":"T1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Closed page T1." {
		t.Errorf("summary=%q", res.Summary)
	}
	if !cleared {
		t.Errorf("ClearDefaultSelection should have been called")
	}
	data := res.Data.(struct {
		TargetID string `json:"targetId"`
		Success  bool   `json:"success"`
	})
	if data.TargetID != "T1" || !data.Success {
		t.Errorf("data=%+v", data)
	}
}

func TestRunClose_SuccessFalse(t *testing.T) {
	env := closeEnv(t, cdp.TargetInfo{TargetID: "T9"}, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.closeTarget" {
			return map[string]any{"success": false}, nil
		}
		return map[string]any{}, nil
	})
	// ClearDefaultSelection left nil to exercise the nil guard.
	res := runClose(context.Background(), json.RawMessage(`{"targetId":"T9"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Close requested for T9 but CDP reported success=false." {
		t.Errorf("summary=%q", res.Summary)
	}
	data := res.Data.(struct {
		TargetID string `json:"targetId"`
		Success  bool   `json:"success"`
	})
	if data.Success {
		t.Errorf("success should be false, data=%+v", data)
	}
}

func TestRunClose_BadParams(t *testing.T) {
	res := runClose(context.Background(), json.RawMessage(`{"targetId":123}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunClose_MissingSelector(t *testing.T) {
	res := runClose(context.Background(), json.RawMessage(`{}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil || res.Err.Code != "missing_selector" {
		t.Fatalf("want missing_selector, got %+v", res.Err)
	}
}

func TestRunClose_ConnFailure(t *testing.T) {
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Conn: func(context.Context) (*cdp.Connection, error) { return nil, errors.New("dial refused") },
	}
	res := runClose(context.Background(), json.RawMessage(`{"targetId":"T1"}`), env)
	if res.Err == nil || res.Err.Code != "open_failed" {
		t.Fatalf("want open_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryConnection {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunClose_AttachFailure(t *testing.T) {
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Conn: func(context.Context) (*cdp.Connection, error) {
			srv := cdptest.NewServer(t, nil)
			return srv.Dial(t), nil
		},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return nil, errors.New("no target")
		},
	}
	res := runClose(context.Background(), json.RawMessage(`{"targetId":"T1"}`), env)
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunClose_CloseTargetFailure(t *testing.T) {
	env := closeEnv(t, cdp.TargetInfo{TargetID: "T1"}, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.closeTarget" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "cannot close"}
		}
		return map[string]any{}, nil
	})
	res := runClose(context.Background(), json.RawMessage(`{"targetId":"T1"}`), env)
	if res.Err == nil || res.Err.Code != "close_failed" {
		t.Fatalf("want close_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryProtocol {
		t.Errorf("category=%q", res.Err.Category)
	}
}

// ---------------------------------------------------------------------------
// page.navigate
// ---------------------------------------------------------------------------

func TestRunNavigate_HappyPath(t *testing.T) {
	env := attachEnv(t, cdp.TargetInfo{TargetID: "T1"}, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Page.navigate" {
			return map[string]any{"frameId": "F1", "loaderId": "L1"}, nil
		}
		return map[string]any{}, nil
	})
	res := runNavigate(context.Background(), json.RawMessage(`{"url":"https://example.com"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Navigated to https://example.com." {
		t.Errorf("summary=%q", res.Summary)
	}
	data := res.Data.(struct {
		FrameID  string `json:"frameId"`
		LoaderID string `json:"loaderId,omitempty"`
		URL      string `json:"url"`
	})
	if data.FrameID != "F1" || data.LoaderID != "L1" || data.URL != "https://example.com" {
		t.Errorf("data=%+v", data)
	}
}

func TestRunNavigate_ErrorText(t *testing.T) {
	env := attachEnv(t, cdp.TargetInfo{TargetID: "T1"}, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Page.navigate" {
			return map[string]any{"frameId": "F1", "errorText": "net::ERR_ABORTED"}, nil
		}
		return map[string]any{}, nil
	})
	res := runNavigate(context.Background(), json.RawMessage(`{"url":"https://bad.example"}`), env)
	if res.Err == nil || res.Err.Code != "navigate_error" {
		t.Fatalf("want navigate_error, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryProtocol {
		t.Errorf("category=%q", res.Err.Category)
	}
	if res.Err.Message != "net::ERR_ABORTED" {
		t.Errorf("message=%q", res.Err.Message)
	}
	if res.Summary != "Navigation to https://bad.example failed: net::ERR_ABORTED" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunNavigate_BadParams(t *testing.T) {
	res := runNavigate(context.Background(), json.RawMessage(`{"url":123}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunNavigate_AttachFailure(t *testing.T) {
	res := runNavigate(context.Background(), json.RawMessage(`{"url":"https://example.com"}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunNavigate_NavigateFailure(t *testing.T) {
	env := attachEnv(t, cdp.TargetInfo{TargetID: "T1"}, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Page.navigate" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "nav broke"}
		}
		return map[string]any{}, nil
	})
	res := runNavigate(context.Background(), json.RawMessage(`{"url":"https://example.com"}`), env)
	if res.Err == nil || res.Err.Code != "navigate_failed" {
		t.Fatalf("want navigate_failed, got %+v", res.Err)
	}
}

// ---------------------------------------------------------------------------
// pages.handleDialog
// ---------------------------------------------------------------------------

// dialogEnv wires Attach plus an EnsureEnabled that drives <domain>.enable
// against the server, mirroring the dispatcher's behavior.
func dialogEnv(t *testing.T, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	conn := srv.Dial(t)
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: conn, Target: cdp.TargetInfo{TargetID: "T1"}, SessionID: "cdp-1"}, nil
		},
		EnsureEnabled: func(ctx context.Context, cdpSID, domain string) error {
			_, err := conn.Send(ctx, cdpSID, domain+".enable", nil)
			return err
		},
	}
}

func TestRunHandleDialog_Accept(t *testing.T) {
	var sawAccept any
	var sawPrompt any
	env := dialogEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Page.handleJavaScriptDialog" {
			var m map[string]any
			if err := json.Unmarshal(params, &m); err != nil {
				t.Fatalf("unmarshal params: %v", err)
			}
			sawAccept = m["accept"]
			sawPrompt = m["promptText"]
		}
		return map[string]any{}, nil
	})
	res := runHandleDialog(context.Background(), json.RawMessage(`{"accept":true,"promptText":"hi"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Accepted native browser dialog." {
		t.Errorf("summary=%q", res.Summary)
	}
	if sawAccept != true {
		t.Errorf("accept arg=%v", sawAccept)
	}
	if sawPrompt != "hi" {
		t.Errorf("promptText arg=%v", sawPrompt)
	}
	data := res.Data.(struct {
		Accepted bool `json:"accepted"`
	})
	if !data.Accepted {
		t.Errorf("data=%+v", data)
	}
}

func TestRunHandleDialog_Dismiss_NoPromptText(t *testing.T) {
	var sawPromptKey bool
	env := dialogEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Page.handleJavaScriptDialog" {
			var m map[string]any
			if err := json.Unmarshal(params, &m); err != nil {
				t.Fatalf("unmarshal params: %v", err)
			}
			_, sawPromptKey = m["promptText"]
		}
		return map[string]any{}, nil
	})
	res := runHandleDialog(context.Background(), json.RawMessage(`{"accept":false}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Dismissed native browser dialog." {
		t.Errorf("summary=%q", res.Summary)
	}
	if sawPromptKey {
		t.Errorf("promptText must be omitted when empty")
	}
	data := res.Data.(struct {
		Accepted bool `json:"accepted"`
	})
	if data.Accepted {
		t.Errorf("data=%+v", data)
	}
}

func TestRunHandleDialog_BadParams(t *testing.T) {
	res := runHandleDialog(context.Background(), json.RawMessage(`{"accept":"yes"}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunHandleDialog_AttachFailure(t *testing.T) {
	res := runHandleDialog(context.Background(), json.RawMessage(`{"accept":true}`), attachErrEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunHandleDialog_EnablePageFailure(t *testing.T) {
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			srv := cdptest.NewServer(t, nil)
			return &tools.AttachedTarget{Conn: srv.Dial(t), Target: cdp.TargetInfo{TargetID: "T1"}, SessionID: "cdp-1"}, nil
		},
		EnsureEnabled: func(context.Context, string, string) error {
			return errors.New("enable failed")
		},
	}
	res := runHandleDialog(context.Background(), json.RawMessage(`{"accept":true}`), env)
	if res.Err == nil || res.Err.Code != "enable_page_failed" {
		t.Fatalf("want enable_page_failed, got %+v", res.Err)
	}
}

func TestRunHandleDialog_HandleDialogFailure(t *testing.T) {
	env := dialogEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Page.handleJavaScriptDialog" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "no dialog"}
		}
		return map[string]any{}, nil
	})
	res := runHandleDialog(context.Background(), json.RawMessage(`{"accept":true}`), env)
	if res.Err == nil || res.Err.Code != "handle_dialog_failed" {
		t.Fatalf("want handle_dialog_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryProtocol {
		t.Errorf("category=%q", res.Err.Category)
	}
}
