package inspecttool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const networkBodyMaxBytes = 5 * 1024 * 1024

const networkBodySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.networkBody parameters",
  "type": "object",
  "properties": {
    "requestId":  {"type": "string", "minLength": 1, "description": "RequestId from a page.networkLog record."},
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "required": ["requestId"],
  "additionalProperties": false
}`

type networkBodyParams struct {
	RequestID  string `json:"requestId"`
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
	Surface    string `json:"surface,omitempty"`
}

// NetworkBody returns the page.networkBody tool, which fetches the response
// body for a previously logged requestId via Network.getResponseBody. Bodies
// over 5 MiB are refused — callers should use cdp.network.* with
// --expose-raw-cdp for streaming retrieval.
func NetworkBody() tools.Tool {
	return tools.Tool{
		Name:        "page.networkBody",
		Description: "Fetch the response body for a requestId obtained from page.networkLog. Hard-capped at 5 MiB; for larger payloads use the raw cdp.network.* tools.",
		Schema:      json.RawMessage(networkBodySchema),
		Run:         runNetworkBody,
	}
}

func runNetworkBody(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p networkBodyParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}

	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}

	if err := env.EnsureEnabled(ctx, att.SessionID, "Network"); err != nil {
		return tools.ClassifyCDPErr("enable_network_failed", err)
	}

	rawResp, err := att.Conn.Send(ctx, att.SessionID, "Network.getResponseBody", map[string]any{
		"requestId": p.RequestID,
	})
	if err != nil {
		return tools.ClassifyCDPErr("get_response_body_failed", err)
	}
	var body struct {
		Body          string `json:"body"`
		Base64Encoded bool   `json:"base64Encoded"`
	}
	if err := json.Unmarshal(rawResp, &body); err != nil {
		return tools.Fail(tools.CategoryProtocol, "body_decode", err.Error(), false)
	}
	if len(body.Body) > networkBodyMaxBytes {
		return tools.Result{
			Err: &tools.EnvelopeError{
				Code:     "body_too_large",
				Message:  "response body exceeds page.networkBody cap; use cdp.network.streamResourceContent (requires --expose-raw-cdp)",
				Category: tools.CategoryUnsupported,
				Details:  map[string]any{"bytes": len(body.Body), "cap": networkBodyMaxBytes},
			},
			Summary: fmt.Sprintf("Response body for %s exceeds %d-byte cap.", p.RequestID, networkBodyMaxBytes),
		}
	}
	encoding := "utf-8"
	if body.Base64Encoded {
		encoding = "base64"
	}
	return tools.OKWithSummary(
		fmt.Sprintf("Fetched %d-byte %s body for %s.", len(body.Body), encoding, p.RequestID),
		struct {
			RequestID     string `json:"requestId"`
			Body          string `json:"body"`
			Base64Encoded bool   `json:"base64Encoded"`
		}{
			RequestID:     p.RequestID,
			Body:          body.Body,
			Base64Encoded: body.Base64Encoded,
		},
	)
}
