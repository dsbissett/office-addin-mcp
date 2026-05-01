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
		InputSchema: t.Schema,
	}
	s.sdk.AddTool(sdkTool, s.makeHandler(t.Name))
}

func (s *Server) makeHandler(toolName string) sdk.ToolHandler {
	return func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		params := req.Params.Arguments
		env := s.disp.Dispatch(ctx, tools.Request{
			Tool:     toolName,
			Params:   params,
			Endpoint: s.currentEndpoint(),
		})
		return envelopeToResult(env), nil
	}
}

// envelopeToResult marshals a tools.Envelope into an MCP CallToolResult.
//
//   - Diagnostics ride in CallToolResult.Meta keyed by DiagnosticsMetaKey, so
//     the agent-facing Content stays clean of observability fields.
//   - On error: IsError is set and Content is one TextContent containing the
//     JSON-encoded EnvelopeError (code, message, category, retryable, details).
//   - On success: when the data payload looks like an inline image
//     (`{mimeType, data}` with image/* mime), we emit an ImageContent block
//     so MCP clients can render it directly. Otherwise the JSON-encoded data
//     rides as a TextContent block.
func envelopeToResult(env tools.Envelope) *sdk.CallToolResult {
	res := &sdk.CallToolResult{
		Meta: sdk.Meta{DiagnosticsMetaKey: env.Diagnostics},
	}
	if env.OK {
		if img, ok := imageFromData(env.Data); ok {
			res.Content = []sdk.Content{img}
			return res
		}
		body, err := json.Marshal(env.Data)
		if err != nil {
			res.IsError = true
			res.Content = []sdk.Content{&sdk.TextContent{Text: marshalFallback(err)}}
			return res
		}
		res.Content = []sdk.Content{&sdk.TextContent{Text: string(body)}}
		return res
	}
	res.IsError = true
	body, err := json.Marshal(env.Error)
	if err != nil {
		res.Content = []sdk.Content{&sdk.TextContent{Text: marshalFallback(err)}}
		return res
	}
	res.Content = []sdk.Content{&sdk.TextContent{Text: string(body)}}
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
