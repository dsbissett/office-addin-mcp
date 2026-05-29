package officetool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// isPowerPointEval reports whether a Runtime.evaluate params blob carries the
// powerpoint.insertTextTable payload (vs excel.readRange). office.embed runs
// both against the same in-process CDP server, so the responder distinguishes
// them by the payload body embedded in the expression. The preamble defines
// both __runExcel and __runPowerPoint, so we key off a token unique to the
// insertTextTable body itself.
func isPowerPointEval(params json.RawMessage) bool {
	var p struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return false
	}
	return strings.Contains(p.Expression, "addTextBox")
}

// embedEnv returns an env whose Attach always succeeds against one in-process
// server. attachErrs, when set per-call-index, makes a specific Attach call fail
// (index 0 = source, index 1 = target).
func embedEnv(t *testing.T, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	return fakeEnv(t, resp)
}

const validEmbedParams = `{"source":{"address":"A1:B2"},"target":{"slideIndex":0}}`

func TestRunEmbed_HappyPath(t *testing.T) {
	env := embedEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method != "Runtime.evaluate" {
			return map[string]any{}, nil
		}
		if isPowerPointEval(params) {
			return cdptest.EvalOffice(map[string]any{
				"slideIndex": float64(0),
				"shapeId":    "sh-1",
				"shapeName":  "TextBox 1",
			}), nil
		}
		return cdptest.EvalOffice(map[string]any{
			"address":     "Sheet1!A1:B2",
			"rowCount":    float64(2),
			"columnCount": float64(2),
			"values":      []any{[]any{"a", "b"}, []any{1, 2}},
		}), nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Embedded 2x2 range onto slide 0." {
		t.Errorf("summary=%q", res.Summary)
	}
	out := res.Data.(map[string]any)
	src := out["source"].(map[string]any)
	if src["address"] != "Sheet1!A1:B2" {
		t.Errorf("source address wrong: %#v", src)
	}
	tgt := out["target"].(map[string]any)
	if tgt["shapeId"] != "sh-1" {
		t.Errorf("target shape wrong: %#v", tgt)
	}
}

func TestRunEmbed_HappyPathWithSheetAndGeometry(t *testing.T) {
	// Exercises the optional source.sheet branch and all four geometry pointers.
	env := embedEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method != "Runtime.evaluate" {
			return map[string]any{}, nil
		}
		if isPowerPointEval(params) {
			return cdptest.EvalOffice(map[string]any{"slideIndex": float64(1)}), nil
		}
		return cdptest.EvalOffice(map[string]any{
			"address":     "Data!A1:C3",
			"rowCount":    float64(3),
			"columnCount": float64(3),
			"values":      []any{[]any{1}, []any{2}, []any{3}},
		}), nil
	})
	params := `{"source":{"address":"A1:C3","sheet":"Data"},` +
		`"target":{"slideIndex":1,"left":10,"top":20,"width":300,"height":200}}`
	res := runEmbed(context.Background(), json.RawMessage(params), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Embedded 3x3 range onto slide 1." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunEmbed_BadParams(t *testing.T) {
	res := runEmbed(context.Background(), json.RawMessage(`{"source":123}`), errEnv(errors.New("x")))
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunEmbed_SourceAttachFailure(t *testing.T) {
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), errEnv(errors.New("no excel")))
	if res.Err == nil || res.Err.Code != "source_attach_failed" {
		t.Fatalf("want source_attach_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category=%q", res.Err.Category)
	}
	if res.Summary != "source attach failed: no excel" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunEmbed_TargetAttachFailure(t *testing.T) {
	// First Attach (source) succeeds against the server; second (target) fails.
	srv := cdptest.NewServer(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"address":     "Sheet1!A1:B2",
				"rowCount":    float64(2),
				"columnCount": float64(2),
				"values":      []any{[]any{"a", "b"}},
			}), nil
		}
		return map[string]any{}, nil
	})
	calls := 0
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			calls++
			if calls == 1 {
				return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
			}
			return nil, errors.New("no powerpoint")
		},
	}
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Code != "target_attach_failed" {
		t.Fatalf("want target_attach_failed, got %+v", res.Err)
	}
	if res.Summary != "target attach failed: no powerpoint" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunEmbed_SourcePayloadOfficeError(t *testing.T) {
	env := embedEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOfficeErr("InvalidArgument", "bad range", map[string]any{"k": "v"}), nil
		}
		return map[string]any{}, nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "InvalidArgument" {
		t.Fatalf("want office_js/InvalidArgument, got %+v", res.Err)
	}
	if res.Summary != "source Excel error: bad range" {
		t.Errorf("summary=%q", res.Summary)
	}
	di, ok := res.Err.Details["debugInfo"].(map[string]any)
	if !ok || di["k"] != "v" {
		t.Errorf("debugInfo not forwarded: %#v", res.Err.Details)
	}
}

func TestRunEmbed_SourcePayloadProtocolException(t *testing.T) {
	env := embedEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return evalException("Uncaught", "TypeError: oops"), nil
		}
		return map[string]any{}, nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Code != "source_payload_protocol_exception" {
		t.Fatalf("want source_payload_protocol_exception, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryProtocol {
		t.Errorf("category=%q", res.Err.Category)
	}
	if !strings.HasPrefix(res.Summary, "source protocol exception:") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunEmbed_SourcePayloadRemoteError(t *testing.T) {
	env := embedEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "gone"}
		}
		return map[string]any{}, nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Code != "source_payload_failed" {
		t.Fatalf("want source_payload_failed, got %+v", res.Err)
	}
	if !strings.HasPrefix(res.Summary, "source Excel payload failed:") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunEmbed_EmptySourceRange(t *testing.T) {
	// Source read succeeds but returns no rows -> empty_source validation error.
	env := embedEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" && !isPowerPointEval(params) {
			return cdptest.EvalOffice(map[string]any{
				"address":     "Sheet1!A1",
				"rowCount":    float64(0),
				"columnCount": float64(0),
				"values":      []any{},
			}), nil
		}
		return map[string]any{}, nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Code != "empty_source" {
		t.Fatalf("want empty_source, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunEmbed_SourceMissingValuesKey(t *testing.T) {
	// values key absent -> type assertion yields empty slice -> empty_source.
	env := embedEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" && !isPowerPointEval(params) {
			return cdptest.EvalOffice(map[string]any{"address": "Sheet1!A1"}), nil
		}
		return map[string]any{}, nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Code != "empty_source" {
		t.Fatalf("want empty_source, got %+v", res.Err)
	}
}

func TestRunEmbed_TargetPayloadOfficeError(t *testing.T) {
	env := embedEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method != "Runtime.evaluate" {
			return map[string]any{}, nil
		}
		if isPowerPointEval(params) {
			return cdptest.EvalOfficeErr("powerpoint_slide_out_of_range", "slide 9 out of range", nil), nil
		}
		return cdptest.EvalOffice(map[string]any{
			"address":     "Sheet1!A1:B2",
			"rowCount":    float64(2),
			"columnCount": float64(2),
			"values":      []any{[]any{"a", "b"}},
		}), nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS ||
		res.Err.Code != "powerpoint_slide_out_of_range" {
		t.Fatalf("want office_js/powerpoint_slide_out_of_range, got %+v", res.Err)
	}
	if res.Summary != "target PowerPoint error: slide 9 out of range" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunEmbed_TargetPayloadRemoteError(t *testing.T) {
	env := embedEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method != "Runtime.evaluate" {
			return map[string]any{}, nil
		}
		if isPowerPointEval(params) {
			return nil, &cdp.RemoteError{Code: -32000, Message: "pp gone"}
		}
		return cdptest.EvalOffice(map[string]any{
			"address":     "Sheet1!A1:B2",
			"rowCount":    float64(2),
			"columnCount": float64(2),
			"values":      []any{[]any{"a", "b"}},
		}), nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Code != "target_payload_failed" {
		t.Fatalf("want target_payload_failed, got %+v", res.Err)
	}
	if !strings.HasPrefix(res.Summary, "target PowerPoint payload failed:") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunEmbed_TargetPayloadProtocolException(t *testing.T) {
	env := embedEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method != "Runtime.evaluate" {
			return map[string]any{}, nil
		}
		if isPowerPointEval(params) {
			return evalException("Uncaught", "RangeError"), nil
		}
		return cdptest.EvalOffice(map[string]any{
			"address":     "Sheet1!A1:B2",
			"rowCount":    float64(2),
			"columnCount": float64(2),
			"values":      []any{[]any{"a", "b"}},
		}), nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Code != "target_payload_protocol_exception" {
		t.Fatalf("want target_payload_protocol_exception, got %+v", res.Err)
	}
	if !strings.HasPrefix(res.Summary, "target protocol exception:") {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunEmbed_SourceDecodeFailure(t *testing.T) {
	// Source payload returns a JSON array, not an object -> json.Unmarshal into
	// map[string]any fails -> decode_source.
	env := embedEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" && !isPowerPointEval(params) {
			return cdptest.EvalOffice([]any{"not", "an", "object"}), nil
		}
		return map[string]any{}, nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Code != "decode_source" {
		t.Fatalf("want decode_source, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunEmbed_TargetDecodeFailure(t *testing.T) {
	// Target payload returns a JSON array -> decode_target.
	env := embedEnv(t, func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
		if method != "Runtime.evaluate" {
			return map[string]any{}, nil
		}
		if isPowerPointEval(params) {
			return cdptest.EvalOffice([]any{1, 2, 3}), nil
		}
		return cdptest.EvalOffice(map[string]any{
			"address":     "Sheet1!A1:B2",
			"rowCount":    float64(2),
			"columnCount": float64(2),
			"values":      []any{[]any{"a", "b"}},
		}), nil
	})
	res := runEmbed(context.Background(), json.RawMessage(validEmbedParams), env)
	if res.Err == nil || res.Err.Code != "decode_target" {
		t.Fatalf("want decode_target, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestEmbed_ToolDefinition(t *testing.T) {
	tool := Embed()
	if tool.Name != "office.embed" {
		t.Errorf("name=%q", tool.Name)
	}
	if tool.Run == nil {
		t.Error("Run is nil")
	}
	// Schema must be valid JSON.
	var schema any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}

// TestEmbed_Annotations pins office.embed's MCP hint flags. It is a mutating
// (not read-only) tool that inserts a new shape (additive, so non-destructive
// and non-idempotent) while orchestrating across arbitrary CDP targets
// (open-world).
func TestEmbed_Annotations(t *testing.T) {
	ann := Embed().Annotations
	if ann == nil {
		t.Fatal("Embed() has no Annotations")
	}
	if ann.ReadOnlyHint {
		t.Error("office.embed must not be ReadOnlyHint (it inserts a shape)")
	}
	if ann.DestructiveHint == nil || *ann.DestructiveHint {
		t.Errorf("office.embed DestructiveHint must be false (additive insert), got %v", ann.DestructiveHint)
	}
	if ann.IdempotentHint {
		t.Error("office.embed must not be IdempotentHint (each call adds another shape)")
	}
	if ann.OpenWorldHint == nil || !*ann.OpenWorldHint {
		t.Errorf("office.embed OpenWorldHint must be true, got %v", ann.OpenWorldHint)
	}
}
