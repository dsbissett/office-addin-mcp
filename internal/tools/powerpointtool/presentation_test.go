package powerpointtool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// ---------- constructor wiring ----------

func TestPresentationConstructors(t *testing.T) {
	cases := []struct {
		name string
		tool tools.Tool
	}{
		{"powerpoint.readPresentation", ReadPresentation()},
		{"powerpoint.readSlides", ReadSlides()},
		{"powerpoint.readSlide", ReadSlide()},
		{"powerpoint.addSlide", AddSlide()},
		{"powerpoint.readSelection", ReadSelection()},
	}
	for _, c := range cases {
		if c.tool.Name != c.name {
			t.Errorf("tool name=%q want %q", c.tool.Name, c.name)
		}
		if c.tool.Run == nil {
			t.Errorf("%s: Run is nil", c.name)
		}
		var js any
		if err := json.Unmarshal(c.tool.Schema, &js); err != nil {
			t.Errorf("%s: schema not valid JSON: %v", c.name, err)
		}
	}
}

// ---------- helper edge cases ----------

func TestHelpersNonMapData(t *testing.T) {
	// arrayLen / stringField / numberField must gracefully handle non-map data.
	if got := arrayLen("not a map", "slides"); got != 0 {
		t.Errorf("arrayLen(non-map)=%d want 0", got)
	}
	if got := arrayLen(map[string]any{"slides": "not an array"}, "slides"); got != 0 {
		t.Errorf("arrayLen(non-array field)=%d want 0", got)
	}
	if got := stringField(42, "title"); got != "" {
		t.Errorf("stringField(non-map)=%q want empty", got)
	}
	if _, ok := numberField([]any{}, "n"); ok {
		t.Error("numberField(non-map) ok=true want false")
	}
}

// ---------- readPresentation ----------

func TestRunReadPresentation_WithTitle(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"title": "Q3 Deck", "slideCount": float64(12)}))
	res := runReadPresentation(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != `Read presentation "Q3 Deck" (12 slide(s)).` {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadPresentation_NoTitle(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"slideCount": float64(4)}))
	res := runReadPresentation(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read presentation (4 slide(s))." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadPresentation_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("GeneralException", "fail"))
	res := runReadPresentation(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js error, got %+v", res.Err)
	}
}

func TestRunReadPresentation_AttachFailure(t *testing.T) {
	res := runReadPresentation(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunReadPresentation_BadParams(t *testing.T) {
	res := runReadPresentation(context.Background(), json.RawMessage(`{"targetId":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- readSlides ----------

func TestRunReadSlides_HappyPath(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"slides": []any{
		map[string]any{"id": "s1"},
		map[string]any{"id": "s2"},
		map[string]any{"id": "s3"},
	}}))
	res := runReadSlides(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Listed 3 slide(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadSlides_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("GeneralException", "fail"))
	res := runReadSlides(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js error, got %+v", res.Err)
	}
}

func TestRunReadSlides_AttachFailure(t *testing.T) {
	res := runReadSlides(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunReadSlides_BadParams(t *testing.T) {
	res := runReadSlides(context.Background(), json.RawMessage(`{"urlPattern":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- readSlide ----------

func TestRunReadSlide_HappyPath(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"shapes": []any{
		map[string]any{"name": "Title 1"},
		map[string]any{"name": "Content Placeholder 2"},
	}}))
	res := runReadSlide(context.Background(), json.RawMessage(`{"slideIndex":1}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read slide 1 (2 shape(s))." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadSlide_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("ItemNotFound", "no slide"))
	res := runReadSlide(context.Background(), json.RawMessage(`{"slideIndex":99}`), env)
	if res.Err == nil || res.Err.Code != "ItemNotFound" {
		t.Fatalf("want ItemNotFound, got %+v", res.Err)
	}
}

func TestRunReadSlide_AttachFailure(t *testing.T) {
	res := runReadSlide(context.Background(), json.RawMessage(`{"slideIndex":0}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunReadSlide_BadParams(t *testing.T) {
	res := runReadSlide(context.Background(), json.RawMessage(`{"slideIndex":"bad"}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- addSlide ----------

func TestRunAddSlide_WithID(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"id": "256#abc"}))
	res := runAddSlide(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Added slide 256#abc." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunAddSlide_NoID(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{}))
	res := runAddSlide(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Added slide." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunAddSlide_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("GeneralException", "fail"))
	res := runAddSlide(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js error, got %+v", res.Err)
	}
}

func TestRunAddSlide_AttachFailure(t *testing.T) {
	res := runAddSlide(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunAddSlide_BadParams(t *testing.T) {
	res := runAddSlide(context.Background(), json.RawMessage(`{"targetId":true}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- readSelection ----------

func TestRunReadSelection_Selected(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"slides": []any{
		map[string]any{"id": "s1"},
		map[string]any{"id": "s2"},
	}}))
	res := runReadSelection(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read 2 selected slide(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadSelection_None(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"slides": []any{}}))
	res := runReadSelection(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "No slides selected." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadSelection_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("GeneralException", "fail"))
	res := runReadSelection(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js error, got %+v", res.Err)
	}
}

func TestRunReadSelection_AttachFailure(t *testing.T) {
	res := runReadSelection(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunReadSelection_BadParams(t *testing.T) {
	res := runReadSelection(context.Background(), json.RawMessage(`{"urlPattern":42}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}
