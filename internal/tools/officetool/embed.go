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
		Run:         runEmbed,
	}
}

func runEmbed(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p embedParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}

	srcSel := tools.TargetSelector{TargetID: p.Source.TargetID, URLPattern: p.Source.URLPattern}
	srcAtt, err := env.Attach(ctx, srcSel)
	if err != nil {
		return failAttach("source", err)
	}
	srcExec := officejs.New(srcAtt.Conn, srcAtt.SessionID)
	srcArgs := map[string]any{"address": p.Source.Address}
	if p.Source.Sheet != "" {
		srcArgs["sheet"] = p.Source.Sheet
	}
	srcRaw, err := srcExec.Run(ctx, "excel.readRange", srcArgs)
	if err != nil {
		return failPayload("source", "Excel", err)
	}
	var srcData map[string]any
	if err := json.Unmarshal(srcRaw, &srcData); err != nil {
		return tools.Fail(tools.CategoryInternal, "decode_source", err.Error(), false)
	}
	values, _ := srcData["values"].([]any)
	if len(values) == 0 {
		return tools.Fail(tools.CategoryValidation, "empty_source", "source range read returned no rows", false)
	}

	tgtSel := tools.TargetSelector{TargetID: p.Target.TargetID, URLPattern: p.Target.URLPattern}
	tgtAtt, err := env.Attach(ctx, tgtSel)
	if err != nil {
		return failAttach("target", err)
	}
	tgtExec := officejs.New(tgtAtt.Conn, tgtAtt.SessionID)
	tgtArgs := map[string]any{
		"slideIndex": p.Target.SlideIndex,
		"rows":       values,
	}
	if p.Target.Left != nil {
		tgtArgs["left"] = *p.Target.Left
	}
	if p.Target.Top != nil {
		tgtArgs["top"] = *p.Target.Top
	}
	if p.Target.Width != nil {
		tgtArgs["width"] = *p.Target.Width
	}
	if p.Target.Height != nil {
		tgtArgs["height"] = *p.Target.Height
	}
	tgtRaw, err := tgtExec.Run(ctx, "powerpoint.insertTextTable", tgtArgs)
	if err != nil {
		return failPayload("target", "PowerPoint", err)
	}
	var tgtData map[string]any
	if err := json.Unmarshal(tgtRaw, &tgtData); err != nil {
		return tools.Fail(tools.CategoryInternal, "decode_target", err.Error(), false)
	}

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
	res.Summary = fmt.Sprintf("Embedded %dx%d range onto slide %d.", int(rowCount), int(colCount), p.Target.SlideIndex)
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
