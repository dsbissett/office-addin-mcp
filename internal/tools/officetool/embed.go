package officetool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const embedSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "office.embed parameters",
  "description": "Read a range from Excel and insert it onto a PowerPoint slide as a text-table shape. Source and target may be different CDP targets reachable from the same debug endpoint.",
  "type": "object",
  "properties": {
    "source": {
      "type": "object",
      "description": "Excel source.",
      "properties": {
        "address":    {"type": "string", "minLength": 1, "description": "Range address, e.g. 'Sheet1!A1:D20'."},
        "sheet":      {"type": "string"},
        "targetId":   {"type": "string", "description": "Excel CDP target id."},
        "urlPattern": {"type": "string", "description": "Substring of Excel taskpane URL."}
      },
      "required": ["address"],
      "additionalProperties": false
    },
    "target": {
      "type": "object",
      "description": "PowerPoint target.",
      "properties": {
        "slideIndex": {"type": "integer", "minimum": 0, "description": "Zero-based destination slide index."},
        "left":       {"type": "number", "description": "Shape left in points."},
        "top":        {"type": "number", "description": "Shape top in points."},
        "width":      {"type": "number", "description": "Shape width in points."},
        "height":     {"type": "number", "description": "Shape height in points."},
        "targetId":   {"type": "string", "description": "PowerPoint CDP target id."},
        "urlPattern": {"type": "string", "description": "Substring of PowerPoint taskpane URL."}
      },
      "required": ["slideIndex"],
      "additionalProperties": false
    }
  },
  "required": ["source", "target"],
  "additionalProperties": false
}`

type embedSource struct {
	Address    string `json:"address"`
	Sheet      string `json:"sheet,omitempty"`
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

type embedTarget struct {
	SlideIndex int      `json:"slideIndex"`
	Left       *float64 `json:"left,omitempty"`
	Top        *float64 `json:"top,omitempty"`
	Width      *float64 `json:"width,omitempty"`
	Height     *float64 `json:"height,omitempty"`
	TargetID   string   `json:"targetId,omitempty"`
	URLPattern string   `json:"urlPattern,omitempty"`
}

type embedParams struct {
	Source embedSource `json:"source"`
	Target embedTarget `json:"target"`
}

// Embed returns the office.embed tool definition.
//
// Limitation: source and target must be reachable from the same CDP debug
// endpoint the server is connected to. In practice that means the user has
// configured Excel and PowerPoint to share a debug port, or has launched a
// fresh add-in that surfaces both. Cross-endpoint embedding is out of scope
// for Phase A.
func Embed() tools.Tool {
	return tools.Tool{
		Name:        "office.embed",
		Description: "Copy values from an Excel range onto a PowerPoint slide as a text table shape. Source/target are independent CDP targets on the same debug endpoint.",
		Schema:      json.RawMessage(embedSchema),
		// Mutating: reads the Excel source but inserts a NEW text-box shape on
		// the PowerPoint slide (additive, not an overwrite/delete), so
		// DestructiveHint is false and the call is not idempotent (each call
		// adds another shape). OpenWorldHint is true: it orchestrates across
		// arbitrary, independent CDP targets on the same debug endpoint.
		Annotations: &tools.Annotations{
			DestructiveHint: tools.BoolPtr(false),
			OpenWorldHint:   tools.BoolPtr(true),
		},
		Run: runEmbed,
	}
}

func runEmbed(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p embedParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}

	srcData, values, fail := embedReadSource(ctx, env, p.Source)
	if fail != nil {
		return *fail
	}

	tgtData, fail := embedWriteTarget(ctx, env, p.Target, values)
	if fail != nil {
		return *fail
	}

	return embedResult(srcData, tgtData, p.Target.SlideIndex)
}

// embedReadSource attaches to the Excel source, reads the requested range, and
// returns the decoded payload plus its row slice. A non-nil *tools.Result is the
// failure to return; on success it is nil.
func embedReadSource(ctx context.Context, env *tools.RunEnv, src embedSource) (map[string]any, []any, *tools.Result) {
	att, err := env.Attach(ctx, tools.TargetSelector{TargetID: src.TargetID, URLPattern: src.URLPattern})
	if err != nil {
		res := failAttach("source", err)
		return nil, nil, &res
	}
	raw, err := officejs.New(att.Conn, att.SessionID).Run(ctx, "excel.readRange", embedSourceArgs(src))
	if err != nil {
		res := failPayload("source", "Excel", err)
		return nil, nil, &res
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		res := tools.Fail(tools.CategoryInternal, "decode_source", err.Error(), false)
		return nil, nil, &res
	}
	values, _ := data["values"].([]any)
	if len(values) == 0 {
		res := tools.Fail(tools.CategoryValidation, "empty_source", "source range read returned no rows", false)
		return nil, nil, &res
	}
	return data, values, nil
}

// embedSourceArgs builds the excel.readRange arguments, including the optional
// sheet selector.
func embedSourceArgs(src embedSource) map[string]any {
	args := map[string]any{"address": src.Address}
	if src.Sheet != "" {
		args["sheet"] = src.Sheet
	}
	return args
}

// embedWriteTarget attaches to the PowerPoint target and inserts the rows as a
// text-table shape. A non-nil *tools.Result is the failure to return.
func embedWriteTarget(ctx context.Context, env *tools.RunEnv, tgt embedTarget, values []any) (map[string]any, *tools.Result) {
	att, err := env.Attach(ctx, tools.TargetSelector{TargetID: tgt.TargetID, URLPattern: tgt.URLPattern})
	if err != nil {
		res := failAttach("target", err)
		return nil, &res
	}
	raw, err := officejs.New(att.Conn, att.SessionID).Run(ctx, "powerpoint.insertTextTable", embedTargetArgs(tgt, values))
	if err != nil {
		res := failPayload("target", "PowerPoint", err)
		return nil, &res
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		res := tools.Fail(tools.CategoryInternal, "decode_target", err.Error(), false)
		return nil, &res
	}
	return data, nil
}

// embedTargetArgs builds the powerpoint.insertTextTable arguments, including any
// supplied geometry overrides.
func embedTargetArgs(tgt embedTarget, values []any) map[string]any {
	args := map[string]any{
		"slideIndex": tgt.SlideIndex,
		"rows":       values,
	}
	if tgt.Left != nil {
		args["left"] = *tgt.Left
	}
	if tgt.Top != nil {
		args["top"] = *tgt.Top
	}
	if tgt.Width != nil {
		args["width"] = *tgt.Width
	}
	if tgt.Height != nil {
		args["height"] = *tgt.Height
	}
	return args
}

// embedResult assembles the success envelope and human summary from the source
// and target payloads.
func embedResult(srcData, tgtData map[string]any, slideIndex int) tools.Result {
	out := map[string]any{
		"source": map[string]any{
			"address":     srcData["address"],
			"rowCount":    srcData["rowCount"],
			"columnCount": srcData["columnCount"],
		},
		"target": tgtData,
	}
	rowCount, _ := srcData["rowCount"].(float64)
	colCount, _ := srcData["columnCount"].(float64)
	res := tools.OK(out)
	res.Summary = fmt.Sprintf("Embedded %dx%d range onto slide %d.", int(rowCount), int(colCount), slideIndex)
	return res
}

func failAttach(role string, err error) tools.Result {
	return tools.Result{
		Err: &tools.EnvelopeError{
			Code:     role + "_attach_failed",
			Message:  err.Error(),
			Category: tools.CategoryNotFound,
		},
		Summary: role + " attach failed: " + err.Error(),
	}
}

func failPayload(role, hostLabel string, err error) tools.Result {
	var oerr *officejs.OfficeError
	if errors.As(err, &oerr) {
		details := map[string]any{}
		if len(oerr.DebugInfo) > 0 {
			var di any
			if json.Unmarshal(oerr.DebugInfo, &di) == nil {
				details["debugInfo"] = di
			}
		}
		res := tools.FailWithDetails(tools.CategoryOfficeJS, codeOrDefault(oerr.Code), oerr.Message, false, details)
		res.Summary = role + " " + hostLabel + " error: " + oerr.Message
		return res
	}
	var pe *officejs.ProtocolException
	if errors.As(err, &pe) {
		return tools.Result{
			Err:     &tools.EnvelopeError{Code: role + "_payload_protocol_exception", Message: pe.Text, Category: tools.CategoryProtocol},
			Summary: role + " protocol exception: " + pe.Text,
		}
	}
	res := tools.ClassifyCDPErr(role+"_payload_failed", err)
	res.Summary = role + " " + hostLabel + " payload failed: " + err.Error()
	return res
}
