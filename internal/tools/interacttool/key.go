package interacttool

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const pressKeySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.pressKey parameters",
  "type": "object",
  "properties": {
    "key":        {"type": "string", "minLength": 1, "description": "Key name (Enter, Tab, Escape, ArrowDown, …) optionally with modifiers (Ctrl+A, Shift+Tab)."},
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "required": ["key"],
  "additionalProperties": false
}`

type pressKeyParams struct {
	Key string `json:"key"`
	selectorCommon
}

// PressKey returns the page.pressKey tool. Parses a "Ctrl+Shift+A" style
// shortcut into modifier flags + key, then dispatches keyDown/keyUp events.
func PressKey() tools.Tool {
	return tools.Tool{
		Name:        "page.pressKey",
		Description: "Press a keyboard key or shortcut (Enter, Tab, Ctrl+A, Shift+Tab) on the focused element via Input.dispatchKeyEvent.",
		Schema:      json.RawMessage(pressKeySchema),
		Annotations: &tools.Annotations{DestructiveHint: tools.BoolPtr(true)},
		Run:         runPressKey,
	}
}

func runPressKey(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p pressKeyParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	att, err := env.Attach(ctx, p.selector())
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}

	mods, key := parseShortcut(p.Key)
	keyInfo := keyDescriptor(key)

	common := keyEventCommon(mods, keyInfo)
	if res := dispatchKeyEvents(ctx, att, common); res.Err != nil {
		return res
	}
	return tools.OKWithSummary(
		"Pressed "+p.Key+".",
		struct {
			Key       string `json:"key"`
			Modifiers int    `json:"modifiers"`
		}{Key: keyInfo.Key, Modifiers: mods},
	)
}

// keyEventCommon builds the shared Input.dispatchKeyEvent fields (everything
// except the keyDown/keyUp "type"). Text is only included for keys that
// produce output, matching Chrome's expectations.
func keyEventCommon(mods int, keyInfo keyInfo) map[string]any {
	common := map[string]any{
		"modifiers":             mods,
		"key":                   keyInfo.Key,
		"code":                  keyInfo.Code,
		"windowsVirtualKeyCode": keyInfo.VK,
		"nativeVirtualKeyCode":  keyInfo.VK,
	}
	if keyInfo.Text != "" {
		common["text"] = keyInfo.Text
	}
	return common
}

// dispatchKeyEvents sends the keyDown then keyUp events. On success it returns
// the zero Result (Err == nil); on CDP failure it returns a classified error.
func dispatchKeyEvents(ctx context.Context, att *tools.AttachedTarget, common map[string]any) tools.Result {
	down := mergeMap(common, map[string]any{"type": "keyDown"})
	up := mergeMap(common, map[string]any{"type": "keyUp"})
	if _, err := att.Conn.Send(ctx, att.SessionID, "Input.dispatchKeyEvent", down); err != nil {
		return tools.ClassifyCDPErr("key_down_failed", err)
	}
	if _, err := att.Conn.Send(ctx, att.SessionID, "Input.dispatchKeyEvent", up); err != nil {
		return tools.ClassifyCDPErr("key_up_failed", err)
	}
	return tools.Result{}
}

// modifierBits maps modifier-key aliases to their CDP modifier bit. Bits
// follow CDP convention: alt=1, ctrl=2, meta=4, shift=8.
var modifierBits = map[string]int{
	"alt":     1,
	"ctrl":    2,
	"control": 2,
	"meta":    4,
	"cmd":     4,
	"command": 4,
	"win":     4,
	"shift":   8,
}

// parseShortcut splits "Ctrl+Shift+A" into a modifier bitmask + the trailing
// key name. Modifier bits follow CDP convention: alt=1, ctrl=2, meta=4, shift=8.
func parseShortcut(s string) (int, string) {
	parts := strings.Split(s, "+")
	mods := 0
	for i := 0; i < len(parts)-1; i++ {
		mods |= modifierBits[strings.ToLower(strings.TrimSpace(parts[i]))]
	}
	return mods, strings.TrimSpace(parts[len(parts)-1])
}

type keyInfo struct {
	Key  string
	Code string
	VK   int
	Text string
}

// namedKeys maps recognized key names to the (key, code, virtualKeyCode, text)
// quadruple Chrome wants. Aliases (Return→Enter, Esc→Escape, " "→Space) share
// the same descriptor value as their canonical name.
var namedKeys = map[string]keyInfo{
	"Enter":      {Key: "Enter", Code: "Enter", VK: 13, Text: "\r"},
	"Return":     {Key: "Enter", Code: "Enter", VK: 13, Text: "\r"},
	"Tab":        {Key: "Tab", Code: "Tab", VK: 9, Text: "\t"},
	"Escape":     {Key: "Escape", Code: "Escape", VK: 27},
	"Esc":        {Key: "Escape", Code: "Escape", VK: 27},
	"Backspace":  {Key: "Backspace", Code: "Backspace", VK: 8},
	"Delete":     {Key: "Delete", Code: "Delete", VK: 46},
	"ArrowUp":    {Key: "ArrowUp", Code: "ArrowUp", VK: 38},
	"ArrowDown":  {Key: "ArrowDown", Code: "ArrowDown", VK: 40},
	"ArrowLeft":  {Key: "ArrowLeft", Code: "ArrowLeft", VK: 37},
	"ArrowRight": {Key: "ArrowRight", Code: "ArrowRight", VK: 39},
	"Home":       {Key: "Home", Code: "Home", VK: 36},
	"End":        {Key: "End", Code: "End", VK: 35},
	"PageUp":     {Key: "PageUp", Code: "PageUp", VK: 33},
	"PageDown":   {Key: "PageDown", Code: "PageDown", VK: 34},
	"Space":      {Key: " ", Code: "Space", VK: 32, Text: " "},
	" ":          {Key: " ", Code: "Space", VK: 32, Text: " "},
}

// keyDescriptor maps common key names to the (key, code, virtualKeyCode)
// triple Chrome wants. Single-character keys default to printing themselves.
func keyDescriptor(name string) keyInfo {
	if info, ok := namedKeys[name]; ok {
		return info
	}
	if len(name) == 1 {
		return singleCharDescriptor(name)
	}
	return keyInfo{Key: name, Code: name}
}

// singleCharDescriptor builds the descriptor for a one-rune key: letters get a
// "Key<X>" code + VK, digits a "Digit<X>" code + VK, and any other symbol
// prints itself with Code==char and no virtual key code.
func singleCharDescriptor(ch string) keyInfo {
	upper := strings.ToUpper(ch)
	if isAsciiLetter(upper) {
		return keyInfo{Key: ch, Code: "Key" + upper, VK: int(upper[0]), Text: ch}
	}
	if isAsciiDigit(upper) {
		return keyInfo{Key: ch, Code: "Digit" + upper, VK: int(upper[0]), Text: ch}
	}
	return keyInfo{Key: ch, Code: ch, Text: ch}
}

// isAsciiLetter reports whether s is a single ASCII letter A-Z.
func isAsciiLetter(s string) bool {
	return len(s) == 1 && s[0] >= 'A' && s[0] <= 'Z'
}

// isAsciiDigit reports whether s is a single ASCII digit 0-9.
func isAsciiDigit(s string) bool {
	return len(s) == 1 && s[0] >= '0' && s[0] <= '9'
}
