package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/gorilla/websocket"
)

// excelEvalResponder takes the Runtime.evaluate frame and returns the JSON
// to include as result.value (i.e. the payload's return shape — either
// {result: ...} or {__officeError: ...}).
type excelEvalResponder func(expr string) string

// fakeExcelBrowser bundles target plumbing with a Runtime.evaluate handler
// that returns a payload-shaped value.
func fakeExcelBrowser(t *testing.T, eval excelEvalResponder) *fakeBrowser {
	t.Helper()
	ft := &fakeTargets{infos: []map[string]any{
		{"targetId": "T1", "type": "page", "url": "https://localhost:3000/taskpane.html"},
	}}
	return newFakeBrowser(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		if ft.handle(t, ws, f) {
			return
		}
		method, _ := f["method"].(string)
		if method != "Runtime.evaluate" {
			return
		}
		params, _ := f["params"].(map[string]any)
		expr, _ := params["expression"].(string)
		valueJSON := eval(expr)
		// CDP returnByValue:true → result.value is the JS-side return.
		writeWSJSON(t, ws, map[string]any{
			"id": f["id"],
			"result": map[string]any{
				"result": map[string]any{
					"type":  "object",
					"value": json.RawMessage(valueJSON),
				},
			},
		})
	})
}

func TestRunCall_ExcelReadRange_Success(t *testing.T) {
	var capturedExpr string
	fb := fakeExcelBrowser(t, func(expr string) string {
		capturedExpr = expr
		return `{"result":{"sheet":"Sheet1","address":"Sheet1!A1:B2","values":[[1,2],[3,4]],"rowCount":2,"columnCount":2}}`
	})
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "excel.readRange",
		"--param", `{"address":"A1:B2","urlPattern":"taskpane"}`,
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	var env tools.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK {
		t.Fatalf("err: %+v", env.Error)
	}
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		Sheet    string  `json:"sheet"`
		Address  string  `json:"address"`
		RowCount int     `json:"rowCount"`
		Values   [][]int `json:"values"`
	}
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		t.Fatal(err)
	}
	if data.Sheet != "Sheet1" || data.Address != "Sheet1!A1:B2" || data.RowCount != 2 {
		t.Errorf("unexpected data: %+v", data)
	}
	for _, want := range []string{"__runExcel", "args.address", `"address":"A1:B2"`} {
		if !strings.Contains(capturedExpr, want) {
			t.Errorf("expression missing %q", want)
		}
	}
}

func TestRunCall_ExcelReadRange_OfficeError(t *testing.T) {
	fb := fakeExcelBrowser(t, func(_ string) string {
		return `{"__officeError":true,"code":"ItemNotFound","message":"Worksheet 'Bogus' not found.","debugInfo":{"errorLocation":"workbook.worksheets.getItem"}}`
	})
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "excel.readRange",
		"--param", `{"address":"A1","sheet":"Bogus","urlPattern":"taskpane"}`,
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Category != tools.CategoryOfficeJS {
		t.Fatalf("expected office_js, got %+v", env.Error)
	}
	if env.Error.Code != "ItemNotFound" {
		t.Errorf("code=%q", env.Error.Code)
	}
	if env.Error.Details["debugInfo"] == nil {
		t.Errorf("expected debugInfo in details, got %+v", env.Error.Details)
	}
}

func TestRunCall_ExcelReadRange_SchemaRejectsMissingAddress(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "excel.readRange",
		"--param", `{}`,
		"--browser-url", "http://127.0.0.1:1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d", code)
	}
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Category != tools.CategoryValidation {
		t.Errorf("expected validation, got %+v", env.Error)
	}
}

func TestRunCall_ExcelWriteRange_RequiresOneOf(t *testing.T) {
	// Schema's anyOf requires one of values/formulas/numberFormat.
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "excel.writeRange",
		"--param", `{"address":"A1"}`,
		"--browser-url", "http://127.0.0.1:1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d", code)
	}
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Category != tools.CategoryValidation {
		t.Errorf("expected validation, got %+v", env.Error)
	}
}

func TestRunCall_ExcelRunScript_PassThrough(t *testing.T) {
	var capturedArgs string
	fb := fakeExcelBrowser(t, func(expr string) string {
		// Capture the args JSON from the parenthesized expression for assertion.
		idx := strings.LastIndex(expr, "})(")
		if idx >= 0 {
			capturedArgs = expr[idx+3 : len(expr)-1]
		}
		return `{"result":{"workbook":"X.xlsx"}}`
	})
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "excel.runScript",
		"--param", `{"script":"const w = context.workbook; w.load('name'); await context.sync(); return {workbook: w.name};","scriptArgs":{"foo":1},"urlPattern":"taskpane"}`,
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(capturedArgs, `"scriptArgs":{"foo":1}`) {
		t.Errorf("expected scriptArgs in payload args, got: %s", capturedArgs)
	}
}
