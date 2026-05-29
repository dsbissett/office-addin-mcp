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
		Annotations: &tools.Annotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: tools.BoolPtr(false)},
		Run:         runNetworkBody,
	}
}

type networkBodyResult struct {
	Body          string `json:"body"`
	Base64Encoded bool   `json:"base64Encoded"`
}

func runNetworkBody(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p networkBodyParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}

	body, fail := fetchNetworkBody(ctx, env, p)
	if fail != nil {
		return *fail
	}
	if len(body.Body) > networkBodyMaxBytes {
		return networkBodyTooLarge(p.RequestID, len(body.Body))
	}
	return tools.OKWithSummary(
		fmt.Sprintf("Fetched %d-byte %s body for %s.", len(body.Body), bodyEncoding(body.Base64Encoded), p.RequestID),
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

// fetchNetworkBody attaches to the selected target, enables Network, and fetches
// and decodes the response body for the requested id. It returns either the
// decoded body or a non-nil failure Result to surface verbatim.
func fetchNetworkBody(ctx context.Context, env *tools.RunEnv, p networkBodyParams) (networkBodyResult, *tools.Result) {
	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return networkBodyResult{}, ptrResult(tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false))
	}
	if err := env.EnsureEnabled(ctx, att.SessionID, "Network"); err != nil {
		return networkBodyResult{}, ptrResult(tools.ClassifyCDPErr("enable_network_failed", err))
	}
	rawResp, err := att.Conn.Send(ctx, att.SessionID, "Network.getResponseBody", map[string]any{
		"requestId": p.RequestID,
	})
	if err != nil {
		return networkBodyResult{}, ptrResult(tools.ClassifyCDPErr("get_response_body_failed", err))
	}
	var body networkBodyResult
	if err := json.Unmarshal(rawResp, &body); err != nil {
		return networkBodyResult{}, ptrResult(tools.Fail(tools.CategoryProtocol, "body_decode", err.Error(), false))
	}
	return body, nil
}

// ptrResult boxes a Result for the (value, *Result) early-exit convention.
func ptrResult(r tools.Result) *tools.Result { return &r }

// bodyEncoding names the wire encoding for a response body.
func bodyEncoding(base64Encoded bool) string {
	if base64Encoded {
		return "base64"
	}
	return "utf-8"
}

// networkBodyTooLarge builds the over-cap refusal result for a response body.
func networkBodyTooLarge(requestID string, n int) tools.Result {
	return tools.Result{
		Err: &tools.EnvelopeError{
			Code:     "body_too_large",
			Message:  "response body exceeds page.networkBody cap; use cdp.network.streamResourceContent (requires --expose-raw-cdp)",
			Category: tools.CategoryUnsupported,
			Details:  map[string]any{"bytes": n, "cap": networkBodyMaxBytes},
		},
		Summary: fmt.Sprintf("Response body for %s exceeds %d-byte cap.", requestID, networkBodyMaxBytes),
	}
}
