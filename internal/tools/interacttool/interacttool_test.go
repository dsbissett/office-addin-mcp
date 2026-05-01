package interacttool

import (
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRegister_AllTools(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	for _, name := range []string{"page.click", "page.fill", "page.hover", "page.typeText", "page.pressKey"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("tool %s not registered", name)
		}
	}
}

func TestParseShortcut_SingleKey(t *testing.T) {
	mods, key := parseShortcut("Enter")
	if mods != 0 {
		t.Errorf("expected 0 modifiers, got %d", mods)
	}
	if key != "Enter" {
		t.Errorf("expected key 'Enter', got %q", key)
	}
}

func TestParseShortcut_CtrlA(t *testing.T) {
	mods, key := parseShortcut("Ctrl+A")
	if mods != 2 {
		t.Errorf("expected modifier=2 (ctrl), got %d", mods)
	}
	if key != "A" {
		t.Errorf("expected key 'A', got %q", key)
	}
}

func TestParseShortcut_CtrlShiftA(t *testing.T) {
	mods, _ := parseShortcut("Ctrl+Shift+A")
	const want = 2 | 8
	if mods != want {
		t.Errorf("expected modifier=%d (ctrl|shift), got %d", want, mods)
	}
}

func TestKeyDescriptor_Letter(t *testing.T) {
	info := keyDescriptor("a")
	if info.Code != "KeyA" {
		t.Errorf("expected code 'KeyA', got %q", info.Code)
	}
	if info.VK != int('A') {
		t.Errorf("expected VK %d, got %d", int('A'), info.VK)
	}
	if info.Text != "a" {
		t.Errorf("expected text 'a', got %q", info.Text)
	}
}

func TestKeyDescriptor_Special(t *testing.T) {
	info := keyDescriptor("ArrowDown")
	if info.VK != 40 {
		t.Errorf("expected ArrowDown VK 40, got %d", info.VK)
	}
}

func TestMergeMap(t *testing.T) {
	a := map[string]any{"x": 1}
	b := map[string]any{"y": 2, "x": 3}
	got := mergeMap(a, b)
	if got["x"] != 3 || got["y"] != 2 {
		t.Errorf("merge wrong: %+v", got)
	}
}
