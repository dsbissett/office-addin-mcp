package macrotool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/recorder"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// newRecorderEnv returns a RunEnv backed by a real recorder.Store rooted at a
// throwaway temp dir, so the record_start / record_stop tools exercise their
// real persistence path without touching the user's macros directory.
func newRecorderEnv(t *testing.T) *tools.RunEnv {
	t.Helper()
	store, err := recorder.New(t.TempDir())
	if err != nil {
		t.Fatalf("recorder.New: %v", err)
	}
	return &tools.RunEnv{Diag: &tools.Diagnostics{}, Recorder: store}
}

// ---- Register --------------------------------------------------------------

func TestRegister_AddsBothRecordTools(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)

	for _, name := range []string{"macro.record_start", "macro.record_stop"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

// ---- RecordStart -----------------------------------------------------------

func TestRecordStart_HappyPath(t *testing.T) {
	tool := RecordStart()
	if tool.Name != "macro.record_start" || !tool.NoSession {
		t.Fatalf("unexpected tool shape: name=%q noSession=%v", tool.Name, tool.NoSession)
	}

	env := newRecorderEnv(t)
	res := tool.Run(context.Background(), json.RawMessage(`{"name":"demo"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Recording started: demo" {
		t.Errorf("summary=%q", res.Summary)
	}
	data, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type=%T", res.Data)
	}
	if data["name"] != "demo" || data["status"] != "recording" {
		t.Errorf("data=%v", data)
	}
}

func TestRecordStart_BadParams(t *testing.T) {
	res := RecordStart().Run(context.Background(), json.RawMessage(`{"name":123}`), newRecorderEnv(t))
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryValidation || res.Err.Code != "invalid_params" {
		t.Errorf("err=%+v, want validation/invalid_params", res.Err)
	}
}

func TestRecordStart_EmptyName(t *testing.T) {
	res := RecordStart().Run(context.Background(), json.RawMessage(`{"name":""}`), newRecorderEnv(t))
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryValidation || res.Err.Code != "empty_name" {
		t.Errorf("err=%+v, want validation/empty_name", res.Err)
	}
}

func TestRecordStart_RecorderUnavailable(t *testing.T) {
	env := &tools.RunEnv{Diag: &tools.Diagnostics{}} // Recorder nil
	res := RecordStart().Run(context.Background(), json.RawMessage(`{"name":"demo"}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryInternal || res.Err.Code != "recording_unavailable" {
		t.Errorf("err=%+v, want internal/recording_unavailable", res.Err)
	}
}

func TestRecordStart_StartRecordingFails(t *testing.T) {
	env := newRecorderEnv(t)
	// First start succeeds; a second start while still active fails inside the
	// store, surfacing as recording_failed.
	if res := RecordStart().Run(context.Background(), json.RawMessage(`{"name":"first"}`), env); res.Err != nil {
		t.Fatalf("first start failed: %+v", res.Err)
	}
	res := RecordStart().Run(context.Background(), json.RawMessage(`{"name":"second"}`), env)
	if res.Err == nil {
		t.Fatal("expected error on second concurrent start")
	}
	if res.Err.Category != tools.CategoryInternal || res.Err.Code != "recording_failed" {
		t.Errorf("err=%+v, want internal/recording_failed", res.Err)
	}
}

// ---- RecordStop ------------------------------------------------------------

func TestRecordStop_HappyPath(t *testing.T) {
	tool := RecordStop()
	if tool.Name != "macro.record_stop" || !tool.NoSession {
		t.Fatalf("unexpected tool shape: name=%q noSession=%v", tool.Name, tool.NoSession)
	}

	env := newRecorderEnv(t)
	store := env.Recorder
	if err := store.StartRecording("demo"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if err := store.Append("excel.runScript", json.RawMessage(`{"script":"x"}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Append("word.runScript", json.RawMessage(`{"script":"y"}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}

	res := tool.Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Recording stopped: demo (2 steps)" {
		t.Errorf("summary=%q", res.Summary)
	}
	data, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type=%T", res.Data)
	}
	if data["name"] != "demo" || data["steps"] != 2 {
		t.Errorf("data=%v", data)
	}
	entries, ok := data["entries"].([]recorder.Entry)
	if !ok {
		t.Fatalf("entries type=%T", data["entries"])
	}
	if len(entries) != 2 || entries[0].Tool != "excel.runScript" {
		t.Errorf("entries=%v", entries)
	}
}

func TestRecordStop_RecorderUnavailable(t *testing.T) {
	env := &tools.RunEnv{Diag: &tools.Diagnostics{}} // Recorder nil
	res := RecordStop().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryInternal || res.Err.Code != "recording_unavailable" {
		t.Errorf("err=%+v, want internal/recording_unavailable", res.Err)
	}
}

func TestRecordStop_StopRecordingFails(t *testing.T) {
	// No active recording — StopRecording returns an error, surfacing as
	// recording_failed.
	env := newRecorderEnv(t)
	res := RecordStop().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil {
		t.Fatal("expected error when not recording")
	}
	if res.Err.Category != tools.CategoryInternal || res.Err.Code != "recording_failed" {
		t.Errorf("err=%+v, want internal/recording_failed", res.Err)
	}
}

// ---- MakeMacroTool ---------------------------------------------------------

// recordingEnv captures progress and log callbacks so the replay path's
// ReportProgress / Logf invocations are exercised.
func recordingEnv() (*tools.RunEnv, *[]string) {
	var logs []string
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Progress: func(current, total float64, message string) {
			logs = append(logs, message)
		},
		Log: func(level, message string) {
			logs = append(logs, level+":"+message)
		},
	}
	return env, &logs
}

func TestMakeMacroTool_Shape(t *testing.T) {
	macro := &recorder.Macro{Name: "demo", Entries: []recorder.Entry{{Tool: "a"}, {Tool: "b"}}}
	tool := MakeMacroTool(macro, func(context.Context, string, json.RawMessage, *tools.RunEnv) tools.Result {
		return tools.OK(nil)
	})
	if tool.Name != "macro.demo" {
		t.Errorf("name=%q", tool.Name)
	}
	if tool.NoSession {
		t.Errorf("replay tools must require a session")
	}
	if tool.Description == "" {
		t.Errorf("empty description")
	}
}

func TestMakeMacroTool_ReplayHappyPath(t *testing.T) {
	macro := &recorder.Macro{
		Name: "build",
		Entries: []recorder.Entry{
			{Tool: "excel.runScript", Params: map[string]any{"script": "one"}},
			{Tool: "word.runScript", Params: map[string]any{"script": "two"}},
			{Tool: "outlook.runScript", Params: map[string]any{"script": "three"}},
		},
	}

	var dispatched []string
	var sawParams []string
	runner := func(_ context.Context, tool string, params json.RawMessage, _ *tools.RunEnv) tools.Result {
		dispatched = append(dispatched, tool)
		sawParams = append(sawParams, string(params))
		return tools.OK(map[string]any{"tool": tool})
	}

	env, logs := recordingEnv()
	res := MakeMacroTool(macro, runner).Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Macro completed: build (3 steps)" {
		t.Errorf("summary=%q", res.Summary)
	}
	data, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type=%T", res.Data)
	}
	if data["macro"] != "build" || data["stepsReplayed"] != 3 {
		t.Errorf("data=%v", data)
	}

	// Each recorded entry dispatched in order.
	wantTools := []string{"excel.runScript", "word.runScript", "outlook.runScript"}
	if len(dispatched) != 3 {
		t.Fatalf("dispatched %d steps, want 3: %v", len(dispatched), dispatched)
	}
	for i, w := range wantTools {
		if dispatched[i] != w {
			t.Errorf("step %d dispatched %q, want %q", i, dispatched[i], w)
		}
	}
	// Params were re-marshaled from the recorded literal.
	if sawParams[0] != `{"script":"one"}` {
		t.Errorf("step 0 params=%q", sawParams[0])
	}

	// Progress + log callbacks fired (3 per-step progress + 3 logs + 1 final).
	if len(*logs) == 0 {
		t.Error("expected progress/log callbacks to fire")
	}
}

func TestMakeMacroTool_EmptyMacro(t *testing.T) {
	macro := &recorder.Macro{Name: "empty"}
	called := false
	runner := func(context.Context, string, json.RawMessage, *tools.RunEnv) tools.Result {
		called = true
		return tools.OK(nil)
	}
	env, _ := recordingEnv()
	res := MakeMacroTool(macro, runner).Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if called {
		t.Error("runner should not be invoked for an empty macro")
	}
	if res.Summary != "Macro completed: empty (0 steps)" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestMakeMacroTool_StepFailsWithDetails(t *testing.T) {
	macro := &recorder.Macro{
		Name: "fails",
		Entries: []recorder.Entry{
			{Tool: "excel.runScript", Params: map[string]any{"script": "ok"}},
			{Tool: "excel.applyDiff", Params: map[string]any{"script": "boom"}},
			{Tool: "word.runScript", Params: map[string]any{"script": "never"}},
		},
	}

	calls := 0
	runner := func(_ context.Context, tool string, _ json.RawMessage, _ *tools.RunEnv) tools.Result {
		calls++
		if tool == "excel.applyDiff" {
			return tools.FailWithDetails(
				tools.CategoryOfficeJS,
				"ItemNotFound",
				"sheet missing",
				true,
				map[string]any{"available_sheets": []string{"Sheet1"}},
			)
		}
		return tools.OK(nil)
	}

	env, _ := recordingEnv()
	res := MakeMacroTool(macro, runner).Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	// Stops on first failure: step 0 ok, step 1 fails, step 2 never runs.
	if calls != 2 {
		t.Errorf("runner called %d times, want 2 (stop on first error)", calls)
	}
	// Failure preserves the underlying category/code/retryable.
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "ItemNotFound" || !res.Err.Retryable {
		t.Errorf("err=%+v, want office_js/ItemNotFound retryable", res.Err)
	}
	// Replay context details merged with the step's own details.
	d := res.Err.Details
	if d == nil {
		t.Fatal("expected details")
	}
	if d["step"] != 1 || d["tool"] != "excel.applyDiff" {
		t.Errorf("details step/tool=%v/%v", d["step"], d["tool"])
	}
	if d["stepsCompleted"] != 1 || d["stepsTotal"] != 3 {
		t.Errorf("details stepsCompleted/stepsTotal=%v/%v", d["stepsCompleted"], d["stepsTotal"])
	}
	if _, ok := d["available_sheets"]; !ok {
		t.Errorf("step-level detail available_sheets not merged: %v", d)
	}
}

func TestMakeMacroTool_StepFailsWithoutDetails(t *testing.T) {
	macro := &recorder.Macro{
		Name:    "fails2",
		Entries: []recorder.Entry{{Tool: "excel.runScript", Params: map[string]any{}}},
	}
	runner := func(context.Context, string, json.RawMessage, *tools.RunEnv) tools.Result {
		// Plain Fail — no Details on the EnvelopeError.
		return tools.Fail(tools.CategoryInternal, "kaboom", "exploded", false)
	}
	env, _ := recordingEnv()
	res := MakeMacroTool(macro, runner).Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryInternal || res.Err.Code != "kaboom" {
		t.Errorf("err=%+v, want internal/kaboom", res.Err)
	}
	// Only the replay-context details are present (no step-level merge).
	d := res.Err.Details
	if d["step"] != 0 || d["tool"] != "excel.runScript" || d["stepsTotal"] != 1 {
		t.Errorf("details=%v", d)
	}
}

// ---- Annotations -----------------------------------------------------------

// derefBool reports the pointer's value, treating nil as the supplied default.
func derefBool(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func TestRecordStart_Annotations(t *testing.T) {
	a := RecordStart().Annotations
	if a == nil {
		t.Fatal("nil annotations")
	}
	if a.ReadOnlyHint {
		t.Errorf("record_start is mutating; ReadOnlyHint must be false")
	}
	if derefBool(a.DestructiveHint, true) {
		t.Errorf("record_start is additive; DestructiveHint must be false")
	}
}

func TestRecordStop_Annotations(t *testing.T) {
	a := RecordStop().Annotations
	if a == nil {
		t.Fatal("nil annotations")
	}
	if a.ReadOnlyHint {
		t.Errorf("record_stop is mutating; ReadOnlyHint must be false")
	}
	if derefBool(a.DestructiveHint, true) {
		t.Errorf("record_stop is additive; DestructiveHint must be false")
	}
}

func TestMakeMacroTool_Annotations(t *testing.T) {
	macro := &recorder.Macro{Name: "demo"}
	a := MakeMacroTool(macro, func(context.Context, string, json.RawMessage, *tools.RunEnv) tools.Result {
		return tools.OK(nil)
	}).Annotations
	if a == nil {
		t.Fatal("nil annotations")
	}
	if a.ReadOnlyHint {
		t.Errorf("replay is mutating; ReadOnlyHint must be false")
	}
	if !derefBool(a.DestructiveHint, false) {
		t.Errorf("replay runs arbitrary recorded calls; DestructiveHint must be true")
	}
	if !derefBool(a.OpenWorldHint, false) {
		t.Errorf("replay touches external Office entities; OpenWorldHint must be true")
	}
}

func TestMakeMacroTool_MarshalParamsFails(t *testing.T) {
	// A channel cannot be JSON-marshaled, so json.Marshal(entry.Params) fails
	// and the tool reports replay_failed before invoking the runner.
	macro := &recorder.Macro{
		Name:    "badparams",
		Entries: []recorder.Entry{{Tool: "excel.runScript", Params: make(chan int)}},
	}
	called := false
	runner := func(context.Context, string, json.RawMessage, *tools.RunEnv) tools.Result {
		called = true
		return tools.OK(nil)
	}
	env, _ := recordingEnv()
	res := MakeMacroTool(macro, runner).Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if called {
		t.Error("runner must not run when params fail to marshal")
	}
	if res.Err.Category != tools.CategoryInternal || res.Err.Code != "replay_failed" {
		t.Errorf("err=%+v, want internal/replay_failed", res.Err)
	}
	if res.Err.Details["step"] != 0 || res.Err.Details["tool"] != "excel.runScript" {
		t.Errorf("details=%v", res.Err.Details)
	}
}
