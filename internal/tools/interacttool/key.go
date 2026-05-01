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

	down := mergeMap(common, map[string]any{"type": "keyDown"})
	up := mergeMap(common, map[string]any{"type": "keyUp"})
	if _, err := att.Conn.Send(ctx, att.SessionID, "Input.dispatchKeyEvent", down); err != nil {
		return tools.ClassifyCDPErr("key_down_failed", err)
	}
	if _, err := att.Conn.Send(ctx, att.SessionID, "Input.dispatchKeyEvent", up); err != nil {
		return tools.ClassifyCDPErr("key_up_failed", err)
	}
	return tools.OK(struct {
		Key       string `json:"key"`
		Modifiers int    `json:"modifiers"`
	}{Key: keyInfo.Key, Modifiers: mods})
}

// parseShortcut splits "Ctrl+Shift+A" into a modifier bitmask + the trailing
// key name. Modifier bits follow CDP convention: alt=1, ctrl=2, meta=4, shift=8.
func parseShortcut(s string) (int, string) {
	parts := strings.Split(s, "+")
	mods := 0
	for i := 0; i < len(parts)-1; i++ {
		switch strings.ToLower(strings.TrimSpace(parts[i])) {
		case "alt":
			mods |= 1
		case "ctrl", "control":
			mods |= 2
		case "meta", "cmd", "command", "win":
			mods |= 4
		case "shift":
			mods |= 8
		}
	}
	return mods, strings.TrimSpace(parts[len(parts)-1])
}

type keyInfo struct {
	Key  string
	Code string
	VK   int
	Text string
}

// keyDescriptor maps common key names to the (key, code, virtualKeyCode)
// triple Chrome wants. Single-character keys default to printing themselves.
func keyDescriptor(name string) keyInfo {
	switch name {
	case "Enter", "Return":
		return keyInfo{Key: "Enter", Code: "Enter", VK: 13, Text: "\r"}
	case "Tab":
		return keyInfo{Key: "Tab", Code: "Tab", VK: 9, Text: "\t"}
	case "Escape", "Esc":
		return keyInfo{Key: "Escape", Code: "Escape", VK: 27}
	case "Backspace":
		return keyInfo{Key: "Backspace", Code: "Backspace", VK: 8}
	case "Delete":
		return keyInfo{Key: "Delete", Code: "Delete", VK: 46}
	case "ArrowUp":
		return keyInfo{Key: "ArrowUp", Code: "ArrowUp", VK: 38}
	case "ArrowDown":
		return keyInfo{Key: "ArrowDown", Code: "ArrowDown", VK: 40}
	case "ArrowLeft":
		return keyInfo{Key: "ArrowLeft", Code: "ArrowLeft", VK: 37}
	case "ArrowRight":
		return keyInfo{Key: "ArrowRight", Code: "ArrowRight", VK: 39}
	case "Home":
		return keyInfo{Key: "Home", Code: "Home", VK: 36}
	case "End":
		return keyInfo{Key: "End", Code: "End", VK: 35}
	case "PageUp":
		return keyInfo{Key: "PageUp", Code: "PageUp", VK: 33}
	case "PageDown":
		return keyInfo{Key: "PageDown", Code: "PageDown", VK: 34}
	case "Space", " ":
		return keyInfo{Key: " ", Code: "Space", VK: 32, Text: " "}
	}
	if len(name) == 1 {
		ch := name
		upper := strings.ToUpper(ch)
		vk := 0
		if len(upper) == 1 && upper[0] >= 'A' && upper[0] <= 'Z' {
			vk = int(upper[0])
			return keyInfo{Key: ch, Code: "Key" + upper, VK: vk, Text: ch}
		}
		if len(upper) == 1 && upper[0] >= '0' && upper[0] <= '9' {
			vk = int(upper[0])
			return keyInfo{Key: ch, Code: "Digit" + upper, VK: vk, Text: ch}
		}
		return keyInfo{Key: ch, Code: ch, Text: ch}
	}
	return keyInfo{Key: name, Code: name}
}
