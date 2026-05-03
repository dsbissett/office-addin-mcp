package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// DiagnosticsMetaKey is the _meta field the adapter uses to carry the
// office-addin-mcp Diagnostics block out-of-band on every CallToolResult.
const DiagnosticsMetaKey = "office-addin-mcp/diagnostics"

// registerTool advertises one tools.Tool to the SDK server. The SDK's raw
// ToolHandler path is used so input validation stays with the existing
// dispatcher (which compiles the same schema with santhosh-tekuri/jsonschema).
func (s *Server) registerTool(t *tools.Tool) {
	sdkTool := &sdk.Tool{
		Name:        t.Name,
		Description: t.Description,
		Title:       t.Title,
		InputSchema: t.Schema,
	}
	if len(t.OutputSchema) > 0 {
		sdkTool.OutputSchema = t.OutputSchema
	}
	if t.Annotations != nil {
		sdkTool.Annotations = &sdk.ToolAnnotations{
			Title:           t.Annotations.Title,
			ReadOnlyHint:    t.Annotations.ReadOnlyHint,
			DestructiveHint: t.Annotations.DestructiveHint,
			IdempotentHint:  t.Annotations.IdempotentHint,
			OpenWorldHint:   t.Annotations.OpenWorldHint,
		}
	}
	s.sdk.AddTool(sdkTool, s.makeHandler(t))
}

func (s *Server) makeHandler(t *tools.Tool) sdk.ToolHandler {
	hasOutputSchema := len(t.OutputSchema) > 0
	toolName := t.Name
	return func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		params := req.Params.Arguments
		env := s.disp.Dispatch(ctx, tools.Request{
			Tool:     toolName,
			Params:   params,
			Endpoint: s.currentEndpoint(),
		})
		return envelopeToResult(env, hasOutputSchema), nil
	}
}

// envelopeToResult marshals a tools.Envelope into an MCP CallToolResult.
//
//   - Diagnostics ride in CallToolResult.Meta keyed by DiagnosticsMetaKey, so
//     the agent-facing Content stays clean of observability fields.
//   - When env.Summary is non-empty, a leading TextContent block carries the
//     terse human-readable line — chat clients display this in the tool's OUT
//     bubble before the JSON payload.
//   - On error: IsError is set and Content is the optional summary block
//     followed by one TextContent containing the JSON-encoded EnvelopeError.
//   - On success: when the data payload looks like an inline image
//     (`{mimeType, data}` with image/* mime), we emit an ImageContent block
//     so MCP clients can render it directly. Otherwise the JSON-encoded data
//     rides as a TextContent block (preceded by the summary block when set).
//   - When emitStructured is true (the tool declared an OutputSchema), the
//     same data is also attached as StructuredContent — MCP clients that
//     support structured output get a typed object; older clients still see
//     the JSON-encoded TextContent.
func envelopeToResult(env tools.Envelope, emitStructured bool) *sdk.CallToolResult {
	res := &sdk.CallToolResult{
		Meta: sdk.Meta{DiagnosticsMetaKey: env.Diagnostics},
	}
	var content []sdk.Content
	if env.Summary != "" {
		content = append(content, &sdk.TextContent{Text: env.Summary})
	}
	if env.OK {
		if img, ok := imageFromData(env.Data); ok {
			res.Content = append(content, img)
			return res
		}
		body, err := json.Marshal(env.Data)
		if err != nil {
			res.IsError = true
			res.Content = append(content, &sdk.TextContent{Text: marshalFallback(err)})
			return res
		}
		res.Content = append(content, &sdk.TextContent{Text: string(body)})
		if emitStructured {
			res.StructuredContent = env.Data
		}
		return res
	}
	res.IsError = true
	body, err := json.Marshal(env.Error)
	if err != nil {
		res.Content = append(content, &sdk.TextContent{Text: marshalFallback(err)})
		return res
	}
	res.Content = append(content, &sdk.TextContent{Text: string(body)})
	return res
}

// imageFromData detects the page.screenshot in-band envelope and converts it
// to an MCP ImageContent block. The data field arrives base64-encoded from
// CDP; ImageContent.Data is []byte that the SDK re-base64-encodes on the
// wire, so we decode first to avoid double-encoding.
func imageFromData(data any) (*sdk.ImageContent, bool) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, false
	}
	var probe struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, false
	}
	if probe.Data == "" || !strings.HasPrefix(probe.MimeType, "image/") {
		return nil, false
	}
	bytes, err := base64.StdEncoding.DecodeString(probe.Data)
	if err != nil {
		return nil, false
	}
	return &sdk.ImageContent{MIMEType: probe.MimeType, Data: bytes}, true
}

func marshalFallback(err error) string {
	msg, _ := json.Marshal(err.Error())
	return `{"code":"marshal_failed","message":` + string(msg) + `,"category":"internal","retryable":false}`
}
