package exceltool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const listChartsSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.listCharts parameters",
  "type": "object",
  "properties": {
    "sheet": {"type": "string", "description": "Worksheet name. Omit to list charts on all worksheets."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ListCharts returns the excel.listCharts tool definition.
func ListCharts() tools.Tool {
	return tools.Tool{
		Name:        "excel.listCharts",
		Description: "List charts across worksheets: name, id, worksheet, type, title, position, size.",
		Schema:      json.RawMessage(listChartsSchema),
		Run:         runListCharts,
	}
}

func runListCharts(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p optionalSheetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	return runPayload(ctx, env, p.selector(), "excel.listCharts", args)
}

const chartInfoSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.chartInfo parameters",
  "type": "object",
  "properties": {
    "sheet": {"type": "string", "minLength": 1, "description": "Worksheet name containing the chart."},
    "name":  {"type": "string", "minLength": 1, "description": "Chart name."},` + targetSelectorBase + `},
  "required": ["sheet", "name"],
  "additionalProperties": false
}`

type chartIDParams struct {
	Sheet string `json:"sheet"`
	Name  string `json:"name"`
	selectorFields
}

// ChartInfo returns the excel.chartInfo tool definition.
func ChartInfo() tools.Tool {
	return tools.Tool{
		Name:        "excel.chartInfo",
		Description: "Detailed information about a chart: type, title, series names, axis titles, position.",
		Schema:      json.RawMessage(chartInfoSchema),
		Run:         runChartInfo,
	}
}

func runChartInfo(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p chartIDParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.chartInfo", map[string]any{
		"sheet": p.Sheet,
		"name":  p.Name,
	})
}

const chartImageSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.chartImage parameters",
  "type": "object",
  "properties": {
    "sheet":  {"type": "string", "minLength": 1, "description": "Worksheet name containing the chart."},
    "name":   {"type": "string", "minLength": 1, "description": "Chart name."},
    "width":  {"type": "integer", "minimum": 1, "description": "Image width in pixels. Defaults to the chart's natural size."},
    "height": {"type": "integer", "minimum": 1, "description": "Image height in pixels. Defaults to the chart's natural size."},` + targetSelectorBase + `},
  "required": ["sheet", "name"],
  "additionalProperties": false
}`

type chartImageParams struct {
	Sheet  string `json:"sheet"`
	Name   string `json:"name"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	selectorFields
}

// ChartImage returns the excel.chartImage tool definition. The payload returns
// {sheet, name, mimeType: "image/png", data: <base64>}; the MCP adapter
// converts that envelope to an ImageContent block automatically.
func ChartImage() tools.Tool {
	return tools.Tool{
		Name:        "excel.chartImage",
		Description: "Render a chart as a PNG image. The MCP response carries it as an ImageContent block.",
		Schema:      json.RawMessage(chartImageSchema),
		Run:         runChartImage,
	}
}

func runChartImage(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p chartImageParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{
		"sheet": p.Sheet,
		"name":  p.Name,
	}
	if p.Width > 0 {
		args["width"] = p.Width
	}
	if p.Height > 0 {
		args["height"] = p.Height
	}
	return runPayload(ctx, env, p.selector(), "excel.chartImage", args)
}
