package cdp

import (
	"context"
	"encoding/json"
	"fmt"
)

// TargetInfo is a subset of CDP TargetInfo.
type TargetInfo struct {
	TargetID         string `json:"targetId"`
	Type             string `json:"type"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	Attached         bool   `json:"attached"`
	BrowserContextID string `json:"browserContextId,omitempty"`
}

// GetTargets calls Target.getTargets.
func (c *Connection) GetTargets(ctx context.Context) ([]TargetInfo, error) {
	raw, err := c.Send(ctx, "", "Target.getTargets", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		TargetInfos []TargetInfo `json:"targetInfos"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode getTargets: %w", err)
	}
	return out.TargetInfos, nil
}

// AttachToTarget attaches with flatten=true and returns the sessionId.
func (c *Connection) AttachToTarget(ctx context.Context, targetID string) (string, error) {
	raw, err := c.Send(ctx, "", "Target.attachToTarget", map[string]any{
		"targetId": targetID,
		"flatten":  true,
	})
	if err != nil {
		return "", err
	}
	var out struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode attachToTarget: %w", err)
	}
	return out.SessionID, nil
}

// CreateTarget calls Target.createTarget and returns the new targetId.
func (c *Connection) CreateTarget(ctx context.Context, url string) (string, error) {
	raw, err := c.Send(ctx, "", "Target.createTarget", map[string]any{"url": url})
	if err != nil {
		return "", err
	}
	var out struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode createTarget: %w", err)
	}
	return out.TargetID, nil
}

// FirstPageTarget returns the first target with type=page that is not a
// devtools:// internal page. Returns ok=false if none found.
func FirstPageTarget(targets []TargetInfo) (TargetInfo, bool) {
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		if isInternalURL(t.URL) {
			continue
		}
		return t, true
	}
	return TargetInfo{}, false
}

func isInternalURL(u string) bool {
	switch {
	case len(u) >= 11 && u[:11] == "devtools://":
		return true
	case len(u) >= 9 && u[:9] == "chrome://":
		return true
	case len(u) >= 7 && u[:7] == "edge://":
		return true
	}
	return false
}
