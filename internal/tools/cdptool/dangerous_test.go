package cdptool

import (
	"context"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/cdptool/generated"
)

// TestDangerousRefusedWithoutFlag confirms a generated dangerous tool
// (Browser.crash) returns an unsupported/dangerous_disabled envelope when
// the dispatcher hasn't been configured with --allow-dangerous-cdp. The
// guard is emitted at the top of every dangerous tool's Run, before any
// CDP work — so we can call it directly with a near-empty RunEnv.
//
// The positive AllowDangerous=true path is implicitly exercised by every
// non-dangerous tool's tests that successfully reach CDP. The dangerous
// guard only changes behavior when the flag is false.
func TestDangerousRefusedWithoutFlag(t *testing.T) {
	tool := generated.NewBrowserCrash()
	env := &tools.RunEnv{AllowDangerous: false}

	res := tool.Run(context.Background(), []byte("{}"), env)
	if res.Err == nil {
		t.Fatal("expected dangerous tool to fail without --allow-dangerous-cdp")
	}
	if res.Err.Code != "dangerous_disabled" {
		t.Errorf("got code %q, want dangerous_disabled", res.Err.Code)
	}
	if res.Err.Category != tools.CategoryUnsupported {
		t.Errorf("got category %q, want %q", res.Err.Category, tools.CategoryUnsupported)
	}
}
