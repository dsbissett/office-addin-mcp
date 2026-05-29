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

// TestAnnotations verifies the MCP hint flags on each interaction tool. Every
// interaction tool mutates the page (mouse/keyboard dispatch or value
// replacement), so none is read-only and all are flagged destructive. fill and
// hover converge to the same end state when repeated, so they are idempotent.
func TestAnnotations(t *testing.T) {
	cases := []struct {
		tool            tools.Tool
		wantDestructive bool
		wantIdempotent  bool
	}{
		{Click(), true, false},
		{Fill(), true, true},
		{Hover(), true, true},
		{TypeText(), true, false},
		{PressKey(), true, false},
	}
	for _, c := range cases {
		a := c.tool.Annotations
		if a == nil {
			t.Errorf("%s: Annotations is nil", c.tool.Name)
			continue
		}
		if a.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint=true, want false", c.tool.Name)
		}
		if a.DestructiveHint == nil || *a.DestructiveHint != c.wantDestructive {
			t.Errorf("%s: DestructiveHint=%v, want %v", c.tool.Name, a.DestructiveHint, c.wantDestructive)
		}
		if a.IdempotentHint != c.wantIdempotent {
			t.Errorf("%s: IdempotentHint=%v, want %v", c.tool.Name, a.IdempotentHint, c.wantIdempotent)
		}
		if a.OpenWorldHint != nil {
			t.Errorf("%s: OpenWorldHint=%v, want nil", c.tool.Name, *a.OpenWorldHint)
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

// TestParseShortcut_AllModifiers exhaustively covers every modifier alias and
// the combined bitmask, plus whitespace trimming around parts.
func TestParseShortcut_AllModifiers(t *testing.T) {
	cases := []struct {
		in       string
		wantMods int
		wantKey  string
	}{
		{"Enter", 0, "Enter"},
		{"Alt+F4", 1, "F4"},
		{"Ctrl+A", 2, "A"},
		{"Control+A", 2, "A"},
		{"Meta+A", 4, "A"},
		{"Cmd+A", 4, "A"},
		{"Command+A", 4, "A"},
		{"Win+A", 4, "A"},
		{"Shift+Tab", 8, "Tab"},
		{"Ctrl+Shift+A", 2 | 8, "A"},
		{"Alt+Ctrl+Meta+Shift+Z", 1 | 2 | 4 | 8, "Z"},
		{" Ctrl + A ", 2, "A"},       // whitespace trimming
		{"Ctrl+Plus+Foo", 2, "Foo"},  // unknown modifier ignored, last part is key
		{"unknownmod+End", 0, "End"}, // unrecognized modifier contributes 0
		{"", 0, ""},                  // empty string => single empty part
	}
	for _, c := range cases {
		mods, key := parseShortcut(c.in)
		if mods != c.wantMods || key != c.wantKey {
			t.Errorf("parseShortcut(%q)=(%d,%q), want (%d,%q)", c.in, mods, key, c.wantMods, c.wantKey)
		}
	}
}

// TestKeyDescriptor_Table covers every named special key and the
// single-character (letter / digit / symbol) and multi-char fallthrough paths.
func TestKeyDescriptor_Table(t *testing.T) {
	cases := []struct {
		name     string
		wantKey  string
		wantCode string
		wantVK   int
		wantText string
	}{
		{"Enter", "Enter", "Enter", 13, "\r"},
		{"Return", "Enter", "Enter", 13, "\r"},
		{"Tab", "Tab", "Tab", 9, "\t"},
		{"Escape", "Escape", "Escape", 27, ""},
		{"Esc", "Escape", "Escape", 27, ""},
		{"Backspace", "Backspace", "Backspace", 8, ""},
		{"Delete", "Delete", "Delete", 46, ""},
		{"ArrowUp", "ArrowUp", "ArrowUp", 38, ""},
		{"ArrowDown", "ArrowDown", "ArrowDown", 40, ""},
		{"ArrowLeft", "ArrowLeft", "ArrowLeft", 37, ""},
		{"ArrowRight", "ArrowRight", "ArrowRight", 39, ""},
		{"Home", "Home", "Home", 36, ""},
		{"End", "End", "End", 35, ""},
		{"PageUp", "PageUp", "PageUp", 33, ""},
		{"PageDown", "PageDown", "PageDown", 34, ""},
		{"Space", " ", "Space", 32, " "},
		{" ", " ", "Space", 32, " "},
		{"A", "A", "KeyA", int('A'), "A"},
		{"z", "z", "KeyZ", int('Z'), "z"},
		{"5", "5", "Digit5", int('5'), "5"},
		{"0", "0", "Digit0", int('0'), "0"},
		{"!", "!", "!", 0, "!"},   // single non-alnum char: Code==char, VK 0
		{"+", "+", "+", 0, "+"},   // single symbol
		{"F1", "F1", "F1", 0, ""}, // multi-char unknown: key/code pass through, no VK/text
	}
	for _, c := range cases {
		info := keyDescriptor(c.name)
		if info.Key != c.wantKey || info.Code != c.wantCode || info.VK != c.wantVK || info.Text != c.wantText {
			t.Errorf("keyDescriptor(%q)=%+v, want {Key:%q Code:%q VK:%d Text:%q}",
				c.name, info, c.wantKey, c.wantCode, c.wantVK, c.wantText)
		}
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
