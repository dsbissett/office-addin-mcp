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
		treq := tools.Request{
			Tool:     toolName,
			Params:   req.Params.Arguments,
			Endpoint: s.currentEndpoint(),
			Log:      logSink(ctx, req.Session, toolName),
		}
		// Progress notifications only flow when the client opted in by sending
		// a progressToken with the call. Skip the sink otherwise so tools that
		// loop over ReportProgress stay silent for non-participating clients.
		if token := req.Params.GetProgressToken(); token != nil {
			treq.Progress = progressSink(ctx, req.Session, token)
		}
		env := s.disp.Dispatch(ctx, treq)
		return envelopeToResult(env, hasOutputSchema), nil
	}
}

// progressSink adapts the tools-layer progress callback onto the SDK session's
// NotifyProgress. Errors are ignored: a dropped progress note must never fail
// the underlying tool call.
func progressSink(ctx context.Context, sess *sdk.ServerSession, token any) func(current, total float64, message string) {
	return func(current, total float64, message string) {
		_ = sess.NotifyProgress(ctx, &sdk.ProgressNotificationParams{
			ProgressToken: token,
			Progress:      current,
			Total:         total,
			Message:       message,
		})
	}
}

// logSink adapts the tools-layer log callback onto the SDK session's Log. The
// SDK no-ops messages below the client's configured level and before any level
// is set, so this is always safe to wire. Errors are ignored.
func logSink(ctx context.Context, sess *sdk.ServerSession, logger string) func(level, message string) {
	return func(level, message string) {
		_ = sess.Log(ctx, &sdk.LoggingMessageParams{
			Level:  sdk.LoggingLevel(level),
			Logger: logger,
			Data:   message,
		})
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
		fillSuccessResult(res, content, env, emitStructured)
		return res
	}
	fillErrorResult(res, content, env.Error)
	return res
}

// fillSuccessResult populates res for a successful envelope: an inline image
// block when the data looks like a screenshot, otherwise the JSON-encoded data
// as a TextContent (plus StructuredContent when the tool declared an output
// schema). A marshal failure flips res into an error result.
func fillSuccessResult(res *sdk.CallToolResult, content []sdk.Content, env tools.Envelope, emitStructured bool) {
	if img, ok := imageFromData(env.Data); ok {
		res.Content = append(content, img)
		return
	}
	body, err := json.Marshal(env.Data)
	if err != nil {
		res.IsError = true
		res.Content = append(content, &sdk.TextContent{Text: marshalFallback(err)})
		return
	}
	res.Content = append(content, &sdk.TextContent{Text: string(body)})
	if emitStructured {
		res.StructuredContent = env.Data
	}
}

// fillErrorResult marks res as an error and appends the JSON-encoded
// EnvelopeError (or a marshal fallback) as a TextContent block.
func fillErrorResult(res *sdk.CallToolResult, content []sdk.Content, envErr *tools.EnvelopeError) {
	res.IsError = true
	body, err := json.Marshal(envErr)
	if err != nil {
		res.Content = append(content, &sdk.TextContent{Text: marshalFallback(err)})
		return
	}
	res.Content = append(content, &sdk.TextContent{Text: string(body)})
}

// imageFromData detects the page.screenshot in-band envelope and converts it
// to an MCP ImageContent block. The data field arrives base64-encoded from
// CDP; ImageContent.Data is []byte that the SDK re-base64-encodes on the
// wire, so we decode first to avoid double-encoding.
func imageFromData(data any) (*sdk.ImageContent, bool) {
	mimeType, encoded, ok := decodeImageProbe(data)
	if !ok {
		return nil, false
	}
	bytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false
	}
	return &sdk.ImageContent{MIMEType: mimeType, Data: bytes}, true
}

// decodeImageProbe re-marshals data and probes it for the screenshot envelope
// shape (`{mimeType, data}` with an image/* mime and non-empty data). It returns
// the mime type, the still-base64-encoded data, and whether the probe matched.
func decodeImageProbe(data any) (mimeType, encoded string, ok bool) {
	body, err := json.Marshal(data)
	if err != nil {
		return "", "", false
	}
	var probe struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return "", "", false
	}
	if probe.Data == "" || !strings.HasPrefix(probe.MimeType, "image/") {
		return "", "", false
	}
	return probe.MimeType, probe.Data, true
}

func marshalFallback(err error) string {
	msg, _ := json.Marshal(err.Error())
	return `{"code":"marshal_failed","message":` + string(msg) + `,"category":"internal","retryable":false}`
}
