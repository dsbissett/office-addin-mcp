package exceltool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const createTableSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.createTable parameters",
  "type": "object",
  "properties": {
    "address":    {"type": "string", "minLength": 1, "description": "Range address that becomes the table body."},
    "sheet":      {"type": "string"},
    "name":       {"type": "string", "description": "Optional table name. Excel auto-generates one if omitted."},
    "hasHeaders": {"type": "boolean", "description": "Whether the first row contains headers (default true)."},` + targetSelectorBase + `},
  "required": ["address"],
  "additionalProperties": false
}`

type createTableParams struct {
	Address    string `json:"address"`
	Sheet      string `json:"sheet,omitempty"`
	Name       string `json:"name,omitempty"`
	HasHeaders *bool  `json:"hasHeaders,omitempty"`
	selectorFields
}

// CreateTable returns the excel.createTable tool definition.
func CreateTable() tools.Tool {
	return tools.Tool{
		Name:        "excel.createTable",
		Description: "Convert a range into an Excel table.",
		Schema:      json.RawMessage(createTableSchema),
		Run:         runCreateTable,
	}
}

func runCreateTable(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p createTableParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{"address": p.Address}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	if p.Name != "" {
		args["name"] = p.Name
	}
	if p.HasHeaders != nil {
		args["hasHeaders"] = *p.HasHeaders
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.createTable", args, func(data any) string {
		name := stringField(data, "name")
		if name == "" {
			name = p.Name
		}
		if name == "" {
			return "Created table at " + p.Address + "."
		}
		return "Created table " + name + " at " + p.Address + "."
	})
}

const listTablesSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.listTables parameters",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ListTables returns the excel.listTables tool definition.
func ListTables() tools.Tool {
	return tools.Tool{
		Name:        "excel.listTables",
		Description: "List all tables (ListObjects) in the workbook with name, worksheet, address, header/total flags, row count, and style.",
		Schema:      json.RawMessage(listTablesSchema),
		Run:         runListTables,
	}
}

func runListTables(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.listTables", map[string]any{}, func(data any) string {
		return fmt.Sprintf("Listed %d table(s).", arrayLen(data, "tables"))
	})
}

const namedTableSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "named table parameters",
  "type": "object",
  "properties": {
    "name": {"type": "string", "minLength": 1, "description": "Table name (ListObject name)."},` + targetSelectorBase + `},
  "required": ["name"],
  "additionalProperties": false
}`

type namedTableParams struct {
	Name string `json:"name"`
	selectorFields
}

// TableInfo returns the excel.tableInfo tool definition.
func TableInfo() tools.Tool {
	return tools.Tool{
		Name:        "excel.tableInfo",
		Description: "Detail for a single table: name, worksheet, address, row count, columns (name + filter criteria), header/total flags, and style.",
		Schema:      json.RawMessage(namedTableSchema),
		Run:         runTableInfo,
	}
}

func runTableInfo(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p namedTableParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.tableInfo", map[string]any{"name": p.Name}, func(data any) string {
		addr := stringField(data, "address")
		if addr != "" {
			return fmt.Sprintf("Table %s at %s.", p.Name, addr)
		}
		return "Returned info for table " + p.Name + "."
	})
}

const tableRowsSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.tableRows parameters",
  "type": "object",
  "properties": {
    "name":           {"type": "string", "minLength": 1, "description": "Table name (ListObject name)."},
    "includeHeaders": {"type": "boolean", "description": "Include the header row names."},` + targetSelectorBase + `},
  "required": ["name"],
  "additionalProperties": false
}`

type tableRowsParams struct {
	Name           string `json:"name"`
	IncludeHeaders bool   `json:"includeHeaders,omitempty"`
	selectorFields
}

// TableRows returns the excel.tableRows tool definition.
func TableRows() tools.Tool {
	return tools.Tool{
		Name:        "excel.tableRows",
		Description: "Data-body values of a table, truncated when row*column exceeds the cell cap.",
		Schema:      json.RawMessage(tableRowsSchema),
		Run:         runTableRows,
	}
}

func runTableRows(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p tableRowsParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{
		"name":           p.Name,
		"includeHeaders": p.IncludeHeaders,
		"maxCells":       maxCells,
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.tableRows", args, func(data any) string {
		rows := numberField(data, "rowCount")
		cols := numberField(data, "columnCount")
		truncSuffix := ""
		if boolField(data, "truncated") {
			truncSuffix = " (truncated)"
		}
		if rows > 0 {
			return fmt.Sprintf("Read %d row(s) x %d column(s) from %s%s.", rows, cols, p.Name, truncSuffix)
		}
		return "Read rows from table " + p.Name + "."
	})
}

// TableFilters returns the excel.tableFilters tool definition.
func TableFilters() tools.Tool {
	return tools.Tool{
		Name:        "excel.tableFilters",
		Description: "Active filter criteria per column for a table. Columns without an active filter have null criteria.",
		Schema:      json.RawMessage(namedTableSchema),
		Run:         runTableFilters,
	}
}

func runTableFilters(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p namedTableParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.tableFilters", map[string]any{"name": p.Name}, func(data any) string {
		return fmt.Sprintf("Returned filters for table %s (%d column(s)).", p.Name, arrayLen(data, "columns"))
	})
}
