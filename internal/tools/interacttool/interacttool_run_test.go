package interacttool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// boxModelResult is the DOM.getBoxModel reply whose content quad centers on
// (15, 25): content = [topLeftX, topLeftY, topRightX, topRightY,
// bottomRightX, bottomRightY, bottomLeftX, bottomLeftY]. The tool averages
// indices 0,4 for x and 1,5 for y -> (10+20)/2=15, (20+30)/2=25.
func boxModelResult() any {
	return map[string]any{
		"model": map[string]any{
			"content": []float64{10, 20, 20, 20, 20, 30, 10, 30},
		},
	}
}

// snap builds a one-node snapshot whose TargetID matches the zero-value
// TargetInfo handed back by the fake Attach below.
func snap(uid string) *session.Snapshot {
	return &session.Snapshot{
		TargetID:     "",
		CDPSessionID: "cdp-1",
		Nodes: map[string]session.SnapshotNode{
			uid: {UID: uid, BackendNodeID: 99, Role: "button", Name: "OK"},
		},
	}
}

// envOpts configures the fake RunEnv built by fakeEnv.
type envOpts struct {
	// snapshot is returned by env.Snapshot. When nil, env.Snapshot returns nil.
	snapshot *session.Snapshot
	// noSnapshotFn makes env.Snapshot itself nil (the "snapshot helper
	// unavailable" branch).
	noSnapshotFn bool
	// ensureErr, when non-nil, is returned by env.EnsureEnabled.
	ensureErr error
}

// fakeEnv returns a RunEnv whose Attach hands the tool a real *cdp.Connection
// backed by an in-process CDP server driven by resp. Snapshot and EnsureEnabled
// are wired from opt so each interaction tool's branches can be exercised.
func fakeEnv(t *testing.T, resp cdptest.Responder, opt envOpts) *tools.RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
		EnsureEnabled: func(context.Context, string, string) error {
			return opt.ensureErr
		},
	}
	if !opt.noSnapshotFn {
		env.Snapshot = func() *session.Snapshot { return opt.snapshot }
	}
	return env
}

// errEnv returns a RunEnv whose Attach always fails — exercises the attach
// failure branch without a server.
func errEnv() *tools.RunEnv {
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return nil, errors.New("no target")
		},
	}
}

// ---------------------------------------------------------------------------
// page.click
// ---------------------------------------------------------------------------

func TestRunClick_HappyPath(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "DOM.getBoxModel" {
			return boxModelResult(), nil
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})

	res := runClick(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.HasPrefix(res.Summary, "Clicked button \"OK\" (u1).") {
		t.Errorf("summary=%q", res.Summary)
	}
	data, ok := res.Data.(struct {
		UID string  `json:"uid"`
		X   float64 `json:"x"`
		Y   float64 `json:"y"`
	})
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if data.X != 15 || data.Y != 25 || data.UID != "u1" {
		t.Errorf("data=%+v want x=15 y=25 uid=u1", data)
	}
}

func TestRunClick_DoubleAndTriple(t *testing.T) {
	resp := func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "DOM.getBoxModel" {
			return boxModelResult(), nil
		}
		return map[string]any{}, nil
	}
	for _, tc := range []struct {
		count int
		verb  string
	}{
		{2, "Double-clicked"},
		{3, "Triple-clicked"},
	} {
		env := fakeEnv(t, resp, envOpts{snapshot: snap("u1")})
		raw := json.RawMessage(`{"uid":"u1","clickCount":` + string(rune('0'+tc.count)) + `,"button":"right"}`)
		res := runClick(context.Background(), raw, env)
		if res.Err != nil {
			t.Fatalf("count %d: unexpected error %+v", tc.count, res.Err)
		}
		if !strings.HasPrefix(res.Summary, tc.verb) {
			t.Errorf("count %d summary=%q want prefix %q", tc.count, res.Summary, tc.verb)
		}
	}
}

func TestRunClick_NodeWithoutName(t *testing.T) {
	// Node.Name == "" exercises the label = node.Role branch.
	s := &session.Snapshot{
		TargetID: "",
		Nodes: map[string]session.SnapshotNode{
			"u1": {UID: "u1", BackendNodeID: 5, Role: "link"},
		},
	}
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "DOM.getBoxModel" {
			return boxModelResult(), nil
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: s})
	res := runClick(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.HasPrefix(res.Summary, "Clicked link (u1).") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunClick_BadParams(t *testing.T) {
	res := runClick(context.Background(), json.RawMessage(`{"uid":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" || res.Err.Category != tools.CategoryValidation {
		t.Fatalf("want param_decode/validation, got %+v", res.Err)
	}
}

func TestRunClick_AttachFailure(t *testing.T) {
	res := runClick(context.Background(), json.RawMessage(`{"uid":"u1"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" || res.Err.Category != tools.CategoryNotFound {
		t.Fatalf("want attach_failed/not_found, got %+v", res.Err)
	}
}

func TestRunClick_LookupFails_UIDNotFound(t *testing.T) {
	env := fakeEnv(t, nil, envOpts{snapshot: snap("other")})
	res := runClick(context.Background(), json.RawMessage(`{"uid":"missing"}`), env)
	if res.Err == nil || res.Err.Code != "uid_not_found" || res.Err.Category != tools.CategoryNotFound {
		t.Fatalf("want uid_not_found/not_found, got %+v", res.Err)
	}
}

func TestRunClick_EnsureDOMFails(t *testing.T) {
	env := fakeEnv(t, nil, envOpts{snapshot: snap("u1"), ensureErr: errors.New("enable boom")})
	res := runClick(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err == nil || res.Err.Code != "enable_dom_failed" {
		t.Fatalf("want enable_dom_failed, got %+v", res.Err)
	}
}

func TestRunClick_BoxModelRemoteError(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "DOM.getBoxModel" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "no box"}
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runClick(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err == nil || res.Err.Code != "get_box_model_failed" {
		t.Fatalf("want get_box_model_failed, got %+v", res.Err)
	}
	// ClassifyCDPErr surfaces the structured CDP error in Details.
	if res.Err.Details == nil || res.Err.Details["cdpError"] == nil {
		t.Errorf("expected cdpError details, got %+v", res.Err.Details)
	}
}

func TestRunClick_BoxQuadTooShort(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "DOM.getBoxModel" {
			return map[string]any{"model": map[string]any{"content": []float64{1, 2, 3}}}, nil
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runClick(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err == nil || res.Err.Code != "box_quad_invalid" || res.Err.Category != tools.CategoryProtocol {
		t.Fatalf("want box_quad_invalid/protocol, got %+v", res.Err)
	}
}

func TestRunClick_MousePressFails(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "DOM.getBoxModel":
			return boxModelResult(), nil
		case "Input.dispatchMouseEvent":
			return nil, &cdp.RemoteError{Code: -32000, Message: "press boom"}
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runClick(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err == nil || res.Err.Code != "mouse_press_failed" {
		t.Fatalf("want mouse_press_failed, got %+v", res.Err)
	}
}

func TestRunClick_MouseReleaseFails(t *testing.T) {
	var seen int
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "DOM.getBoxModel":
			return boxModelResult(), nil
		case "Input.dispatchMouseEvent":
			seen++
			if seen >= 2 { // first = pressed ok, second = released fails
				return nil, &cdp.RemoteError{Code: -32000, Message: "release boom"}
			}
			return map[string]any{}, nil
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runClick(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err == nil || res.Err.Code != "mouse_release_failed" {
		t.Fatalf("want mouse_release_failed, got %+v", res.Err)
	}
}

// ---------------------------------------------------------------------------
// page.hover
// ---------------------------------------------------------------------------

func TestRunHover_HappyPath(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "DOM.getBoxModel" {
			return boxModelResult(), nil
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runHover(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Hovered u1." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunHover_BadParams(t *testing.T) {
	res := runHover(context.Background(), json.RawMessage(`{"uid":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunHover_AttachFailure(t *testing.T) {
	res := runHover(context.Background(), json.RawMessage(`{"uid":"u1"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunHover_LookupFails(t *testing.T) {
	// nil snapshot -> no_snapshot
	env := fakeEnv(t, nil, envOpts{snapshot: nil})
	res := runHover(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err == nil || res.Err.Code != "no_snapshot" || res.Err.Category != tools.CategoryNotFound {
		t.Fatalf("want no_snapshot/not_found, got %+v", res.Err)
	}
}

func TestRunHover_MouseMoveFails(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "DOM.getBoxModel":
			return boxModelResult(), nil
		case "Input.dispatchMouseEvent":
			return nil, &cdp.RemoteError{Code: -32000, Message: "move boom"}
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runHover(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err == nil || res.Err.Code != "mouse_move_failed" {
		t.Fatalf("want mouse_move_failed, got %+v", res.Err)
	}
}

// ---------------------------------------------------------------------------
// page.typeText
// ---------------------------------------------------------------------------

func TestRunTypeText_HappyPath(t *testing.T) {
	env := fakeEnv(t, nil, envOpts{})
	res := runTypeText(context.Background(), json.RawMessage(`{"text":"hello"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Typed 5 character(s) at focused element." {
		t.Errorf("summary=%q", res.Summary)
	}
	data, ok := res.Data.(struct {
		Text string `json:"text"`
	})
	if !ok || data.Text != "hello" {
		t.Errorf("data=%+v", res.Data)
	}
}

func TestRunTypeText_BadParams(t *testing.T) {
	res := runTypeText(context.Background(), json.RawMessage(`{"text":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunTypeText_AttachFailure(t *testing.T) {
	res := runTypeText(context.Background(), json.RawMessage(`{"text":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunTypeText_InsertFails(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Input.insertText" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "insert boom"}
		}
		return map[string]any{}, nil
	}, envOpts{})
	res := runTypeText(context.Background(), json.RawMessage(`{"text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "insert_text_failed" {
		t.Fatalf("want insert_text_failed, got %+v", res.Err)
	}
}

// ---------------------------------------------------------------------------
// page.pressKey
// ---------------------------------------------------------------------------

func TestRunPressKey_HappyPath(t *testing.T) {
	env := fakeEnv(t, nil, envOpts{})
	res := runPressKey(context.Background(), json.RawMessage(`{"key":"Ctrl+A"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Pressed Ctrl+A." {
		t.Errorf("summary=%q", res.Summary)
	}
	data, ok := res.Data.(struct {
		Key       string `json:"key"`
		Modifiers int    `json:"modifiers"`
	})
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if data.Key != "A" || data.Modifiers != 2 {
		t.Errorf("data=%+v want key=A mods=2", data)
	}
}

func TestRunPressKey_PlainKeyWithText(t *testing.T) {
	// "Enter" has Text set, exercising the keyInfo.Text != "" branch.
	env := fakeEnv(t, nil, envOpts{})
	res := runPressKey(context.Background(), json.RawMessage(`{"key":"Enter"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Pressed Enter." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunPressKey_NoTextKey(t *testing.T) {
	// "Escape" has no Text, exercising the omit-text branch.
	env := fakeEnv(t, nil, envOpts{})
	res := runPressKey(context.Background(), json.RawMessage(`{"key":"Escape"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

func TestRunPressKey_BadParams(t *testing.T) {
	res := runPressKey(context.Background(), json.RawMessage(`{"key":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunPressKey_AttachFailure(t *testing.T) {
	res := runPressKey(context.Background(), json.RawMessage(`{"key":"Enter"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunPressKey_KeyDownFails(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Input.dispatchKeyEvent" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "down boom"}
		}
		return map[string]any{}, nil
	}, envOpts{})
	res := runPressKey(context.Background(), json.RawMessage(`{"key":"Enter"}`), env)
	if res.Err == nil || res.Err.Code != "key_down_failed" {
		t.Fatalf("want key_down_failed, got %+v", res.Err)
	}
}

func TestRunPressKey_KeyUpFails(t *testing.T) {
	var seen int
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Input.dispatchKeyEvent" {
			seen++
			if seen >= 2 { // first = keyDown ok, second = keyUp fails
				return nil, &cdp.RemoteError{Code: -32000, Message: "up boom"}
			}
			return map[string]any{}, nil
		}
		return map[string]any{}, nil
	}, envOpts{})
	res := runPressKey(context.Background(), json.RawMessage(`{"key":"Enter"}`), env)
	if res.Err == nil || res.Err.Code != "key_up_failed" {
		t.Fatalf("want key_up_failed, got %+v", res.Err)
	}
}

// ---------------------------------------------------------------------------
// page.fill
// ---------------------------------------------------------------------------

// fillResponder drives a full happy-path fill for a non-select element.
// tagName controls what the tagName probe returns ("input" vs "select").
func fillResponder(tagName string) cdptest.Responder {
	return func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "DOM.resolveNode":
			return map[string]any{"object": map[string]any{"objectId": "obj-1"}}, nil
		case "Runtime.callFunctionOn":
			// Distinguish the tagName probe from the select-set call by the
			// presence of an "arguments" array.
			var p struct {
				Arguments []any `json:"arguments"`
			}
			_ = json.Unmarshal(params, &p)
			if len(p.Arguments) == 0 {
				return cdptest.Eval(tagName), nil
			}
			return cdptest.Eval(tagName), nil
		case "Runtime.evaluate":
			return cdptest.Eval(nil), nil
		default:
			return map[string]any{}, nil
		}
	}
}

func TestRunFill_InputHappyPath(t *testing.T) {
	env := fakeEnv(t, fillResponder("input"), envOpts{snapshot: snap("u1")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"abc"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Filled u1 with 3 character(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
	data, ok := res.Data.(struct {
		UID  string `json:"uid"`
		Text string `json:"text"`
		Mode string `json:"mode"`
	})
	if !ok || data.Mode != "input" || data.Text != "abc" {
		t.Errorf("data=%+v", res.Data)
	}
}

func TestRunFill_SelectHappyPath(t *testing.T) {
	env := fakeEnv(t, fillResponder("select"), envOpts{snapshot: snap("u1")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"opt"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Set <select> u1 to opt." {
		t.Errorf("summary=%q", res.Summary)
	}
	data, ok := res.Data.(struct {
		UID  string `json:"uid"`
		Text string `json:"text"`
		Mode string `json:"mode"`
	})
	if !ok || data.Mode != "select" {
		t.Errorf("data=%+v", res.Data)
	}
}

func TestRunFill_BadParams(t *testing.T) {
	res := runFill(context.Background(), json.RawMessage(`{"uid":123,"text":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunFill_AttachFailure(t *testing.T) {
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunFill_LookupFails(t *testing.T) {
	env := fakeEnv(t, nil, envOpts{snapshot: snap("other")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"missing","text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "uid_not_found" {
		t.Fatalf("want uid_not_found, got %+v", res.Err)
	}
}

func TestRunFill_EnsureDOMFails(t *testing.T) {
	env := fakeEnv(t, fillResponder("input"), envOpts{snapshot: snap("u1"), ensureErr: errors.New("boom")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "enable_dom_failed" {
		t.Fatalf("want enable_dom_failed, got %+v", res.Err)
	}
}

func TestRunFill_ResolveNodeFails(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "DOM.resolveNode" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "resolve boom"}
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "resolve_node_failed" {
		t.Fatalf("want resolve_node_failed, got %+v", res.Err)
	}
}

func TestRunFill_ResolveNoObject(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "DOM.resolveNode" {
			return map[string]any{"object": map[string]any{"objectId": ""}}, nil
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "resolve_no_object" || res.Err.Category != tools.CategoryProtocol {
		t.Fatalf("want resolve_no_object/protocol, got %+v", res.Err)
	}
}

func TestRunFill_TagNameProbeFails(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "DOM.resolveNode":
			return map[string]any{"object": map[string]any{"objectId": "obj-1"}}, nil
		case "Runtime.callFunctionOn":
			return nil, &cdp.RemoteError{Code: -32000, Message: "tag boom"}
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "tagname_failed" {
		t.Fatalf("want tagname_failed, got %+v", res.Err)
	}
}

func TestRunFill_SelectSetFails(t *testing.T) {
	env := fakeEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "DOM.resolveNode":
			return map[string]any{"object": map[string]any{"objectId": "obj-1"}}, nil
		case "Runtime.callFunctionOn":
			var p struct {
				Arguments []any `json:"arguments"`
			}
			_ = json.Unmarshal(params, &p)
			if len(p.Arguments) == 0 {
				return cdptest.Eval("select"), nil // tagName probe
			}
			return nil, &cdp.RemoteError{Code: -32000, Message: "set boom"} // select-set
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "select_set_failed" {
		t.Fatalf("want select_set_failed, got %+v", res.Err)
	}
}

func TestRunFill_FocusFails(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "DOM.resolveNode":
			return map[string]any{"object": map[string]any{"objectId": "obj-1"}}, nil
		case "Runtime.callFunctionOn":
			return cdptest.Eval("input"), nil
		case "DOM.focus":
			return nil, &cdp.RemoteError{Code: -32000, Message: "focus boom"}
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "focus_failed" {
		t.Fatalf("want focus_failed, got %+v", res.Err)
	}
}

func TestRunFill_ClearFails(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "DOM.resolveNode":
			return map[string]any{"object": map[string]any{"objectId": "obj-1"}}, nil
		case "Runtime.callFunctionOn":
			return cdptest.Eval("input"), nil
		case "DOM.focus":
			return map[string]any{}, nil
		case "Runtime.evaluate":
			return nil, &cdp.RemoteError{Code: -32000, Message: "clear boom"}
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "clear_failed" {
		t.Fatalf("want clear_failed, got %+v", res.Err)
	}
}

func TestRunFill_InsertFails(t *testing.T) {
	env := fakeEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "DOM.resolveNode":
			return map[string]any{"object": map[string]any{"objectId": "obj-1"}}, nil
		case "Runtime.callFunctionOn":
			return cdptest.Eval("input"), nil
		case "DOM.focus":
			return map[string]any{}, nil
		case "Runtime.evaluate":
			return cdptest.Eval(nil), nil
		case "Input.insertText":
			return nil, &cdp.RemoteError{Code: -32000, Message: "insert boom"}
		}
		return map[string]any{}, nil
	}, envOpts{snapshot: snap("u1")})
	res := runFill(context.Background(), json.RawMessage(`{"uid":"u1","text":"x"}`), env)
	if res.Err == nil || res.Err.Code != "insert_text_failed" {
		t.Fatalf("want insert_text_failed, got %+v", res.Err)
	}
}

// ---------------------------------------------------------------------------
// lookupNode branches not otherwise hit
// ---------------------------------------------------------------------------

func TestLookupNode_NoSnapshotHelper(t *testing.T) {
	// env.Snapshot == nil -> no_snapshot_runtime / unsupported.
	env := fakeEnv(t, nil, envOpts{noSnapshotFn: true})
	res := runHover(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err == nil || res.Err.Code != "no_snapshot_runtime" || res.Err.Category != tools.CategoryUnsupported {
		t.Fatalf("want no_snapshot_runtime/unsupported, got %+v", res.Err)
	}
}

func TestLookupNode_TargetMismatch(t *testing.T) {
	// Snapshot taken on a different target than the (zero-value) attached one.
	s := &session.Snapshot{
		TargetID: "T-other",
		Nodes:    map[string]session.SnapshotNode{"u1": {UID: "u1", BackendNodeID: 1}},
	}
	env := fakeEnv(t, nil, envOpts{snapshot: s})
	res := runHover(context.Background(), json.RawMessage(`{"uid":"u1"}`), env)
	if res.Err == nil || res.Err.Code != "snapshot_target_mismatch" || res.Err.Category != tools.CategoryNotFound {
		t.Fatalf("want snapshot_target_mismatch/not_found, got %+v", res.Err)
	}
}

// ---------------------------------------------------------------------------
// makeSelector / selectorCommon.selector (via params with selector fields)
// ---------------------------------------------------------------------------

func TestSelectorCommon_Selector(t *testing.T) {
	sc := selectorCommon{TargetID: "T1", URLPattern: "taskpane.html", Surface: "taskpane"}
	sel := sc.selector()
	if sel.TargetID != "T1" || sel.URLPattern != "taskpane.html" || string(sel.Surface) != "taskpane" {
		t.Errorf("selector=%+v", sel)
	}
}

func TestMakeSelector(t *testing.T) {
	sel := makeSelector("T2", "url", "dialog")
	if sel.TargetID != "T2" || sel.URLPattern != "url" || string(sel.Surface) != "dialog" {
		t.Errorf("selector=%+v", sel)
	}
}

func TestRunClick_PassesSelectorFields(t *testing.T) {
	// Confirms selector fields flow through Attach (selectorCommon.selector path).
	var gotSel tools.TargetSelector
	srv := cdptest.NewServer(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "DOM.getBoxModel" {
			return boxModelResult(), nil
		}
		return map[string]any{}, nil
	})
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(_ context.Context, sel tools.TargetSelector) (*tools.AttachedTarget, error) {
			gotSel = sel
			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
		EnsureEnabled: func(context.Context, string, string) error { return nil },
		Snapshot:      func() *session.Snapshot { return snap("u1") },
	}
	res := runClick(context.Background(),
		json.RawMessage(`{"uid":"u1","targetId":"T9","urlPattern":"foo","surface":"dialog"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if gotSel.TargetID != "T9" || gotSel.URLPattern != "foo" || string(gotSel.Surface) != "dialog" {
		t.Errorf("selector passed to Attach=%+v", gotSel)
	}
}
