package cdp

import (
	"context"
	"encoding/json"
	"fmt"
)

// PageNavigateResult mirrors the Page.navigate response.
type PageNavigateResult struct {
	FrameID   string `json:"frameId"`
	LoaderID  string `json:"loaderId,omitempty"`
	ErrorText string `json:"errorText,omitempty"`
}

// PageNavigate calls Page.navigate inside the given session. sessionID must be
// non-empty — Page.navigate is target-scoped.
func (c *Connection) PageNavigate(ctx context.Context, sessionID, url string) (*PageNavigateResult, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("cdp pageNavigate: sessionID is required")
	}
	if url == "" {
		return nil, fmt.Errorf("cdp pageNavigate: url is required")
	}
	raw, err := c.Send(ctx, sessionID, "Page.navigate", map[string]any{"url": url})
	if err != nil {
		return nil, err
	}
	var out PageNavigateResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode page.navigate: %w", err)
	}
	return &out, nil
}
