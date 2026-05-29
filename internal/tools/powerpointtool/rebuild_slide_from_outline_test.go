package powerpointtool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRunRebuildSlideFromOutline_TitleAndBullets(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"bulletsSet": float64(3)}))
	res := runRebuildSlideFromOutline(context.Background(),
		json.RawMessage(`{"slideIndex":2,"title":"Hello","bullets":["a","b","c"]}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Rebuilt slide 2 (title true, 3 bullet(s))." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunRebuildSlideFromOutline_TitleOnly(t *testing.T) {
	// No bulletsSet field in the payload data → bullets defaults to 0.
	env := fakeEnv(t, officeOK(map[string]any{}))
	res := runRebuildSlideFromOutline(context.Background(),
		json.RawMessage(`{"slideIndex":0,"title":"Only Title"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Rebuilt slide 0 (title true, 0 bullet(s))." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunRebuildSlideFromOutline_BulletsOnly(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"bulletsSet": float64(2)}))
	res := runRebuildSlideFromOutline(context.Background(),
		json.RawMessage(`{"slideIndex":5,"bullets":["x","y"]}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Rebuilt slide 5 (title false, 2 bullet(s))." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunRebuildSlideFromOutline_NothingToSet(t *testing.T) {
	res := runRebuildSlideFromOutline(context.Background(),
		json.RawMessage(`{"slideIndex":1}`), errEnv())
	if res.Err == nil || res.Err.Code != "nothing_to_set" {
		t.Fatalf("want nothing_to_set, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category=%q want validation", res.Err.Category)
	}
	if !strings.Contains(res.Err.Message, "at least one of") {
		t.Errorf("message=%q", res.Err.Message)
	}
}

func TestRunRebuildSlideFromOutline_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("ItemNotFound", "no such slide"))
	res := runRebuildSlideFromOutline(context.Background(),
		json.RawMessage(`{"slideIndex":99,"title":"x"}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "ItemNotFound" {
		t.Errorf("err=%+v, want office_js/ItemNotFound", res.Err)
	}
}

func TestRunRebuildSlideFromOutline_AttachFailure(t *testing.T) {
	res := runRebuildSlideFromOutline(context.Background(),
		json.RawMessage(`{"slideIndex":0,"title":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunRebuildSlideFromOutline_BadParams(t *testing.T) {
	res := runRebuildSlideFromOutline(context.Background(),
		json.RawMessage(`{"slideIndex":"nope"}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}
