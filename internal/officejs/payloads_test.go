package officejs

import (
	"strings"
	"testing"
)

func TestFileToToolName(t *testing.T) {
	cases := map[string]string{
		"excel_read_range.js":           "excel.readRange",
		"excel_get_active_worksheet.js": "excel.getActiveWorksheet",
		"excel_run_script.js":           "excel.runScript",
		"excel_a.js":                    "excel.a",
	}
	for in, want := range cases {
		got, err := fileToToolName(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}

func TestFileToToolName_Rejects(t *testing.T) {
	bad := []string{"_preamble.js", "no_underscore.js", "trailing_.js"}
	bad[1] = "noseparator.js" // overwrite to hit the no-underscore branch
	for _, in := range bad {
		if _, err := fileToToolName(in); err == nil {
			t.Errorf("%s: expected error", in)
		}
	}
}

func TestPreloadAndNames(t *testing.T) {
	if err := Preload(); err != nil {
		t.Fatalf("preload: %v", err)
	}
	names := Names()
	want := []string{
		"excel.readRange", "excel.writeRange",
		"excel.listWorksheets", "excel.getActiveWorksheet",
		"excel.activateWorksheet", "excel.createWorksheet", "excel.deleteWorksheet",
		"excel.getSelectedRange", "excel.setSelectedRange",
		"excel.runScript", "excel.createTable",
	}
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	for _, n := range want {
		if !have[n] {
			t.Errorf("missing payload %q (loaded: %v)", n, names)
		}
	}
}

func TestRequirementsParsed(t *testing.T) {
	if err := Preload(); err != nil {
		t.Fatalf("preload: %v", err)
	}
	reqs, err := Requirements("excel.readRange")
	if err != nil {
		t.Fatalf("requirements: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Set != "ExcelApi" || reqs[0].Version != "1.1" {
		t.Errorf("got %+v, want [{ExcelApi 1.1}]", reqs)
	}
	reqs, err = Requirements("excel.activateWorksheet")
	if err != nil {
		t.Fatalf("requirements: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Version != "1.7" {
		t.Errorf("got %+v, want version 1.7", reqs)
	}
}

func TestPreambleEmbedded(t *testing.T) {
	if err := Preload(); err != nil {
		t.Fatalf("preload: %v", err)
	}
	pre, err := preamble()
	if err != nil {
		t.Fatalf("preamble: %v", err)
	}
	for _, want := range []string{"__officeError", "__ensureOffice", "__requireSet", "__runExcel", "Office.onReady"} {
		if !strings.Contains(pre, want) {
			t.Errorf("preamble missing %q", want)
		}
	}
}
