package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

func TestOK(t *testing.T) {
	res := OK(map[string]any{"x": 1})
	if res.Err != nil {
		t.Fatalf("OK should have nil Err, got %+v", res.Err)
	}
	if res.Summary != "" {
		t.Errorf("OK summary=%q want empty", res.Summary)
	}
	m, ok := res.Data.(map[string]any)
	if !ok || m["x"] != 1 {
		t.Errorf("Data=%v", res.Data)
	}
}

func TestOKWithSummary(t *testing.T) {
	res := OKWithSummary("did the thing", 42)
	if res.Err != nil {
		t.Fatalf("OKWithSummary should have nil Err, got %+v", res.Err)
	}
	if res.Summary != "did the thing" {
		t.Errorf("Summary=%q", res.Summary)
	}
	if res.Data != 42 {
		t.Errorf("Data=%v want 42", res.Data)
	}
}

func TestFail(t *testing.T) {
	res := Fail(CategoryValidation, "bad_input", "nope", true)
	if res.Data != nil {
		t.Errorf("Fail Data=%v want nil", res.Data)
	}
	if res.Err == nil {
		t.Fatal("Fail should set Err")
	}
	if res.Err.Code != "bad_input" || res.Err.Category != CategoryValidation ||
		res.Err.Message != "nope" || !res.Err.Retryable {
		t.Errorf("Err=%+v", res.Err)
	}
	if res.Err.Details != nil {
		t.Errorf("Fail should not set Details, got %v", res.Err.Details)
	}
}

func TestFailWithDetails(t *testing.T) {
	details := map[string]any{"probedEndpoint": "http://x"}
	res := FailWithDetails(CategoryConnection, "dial", "down", false, details)
	if res.Err == nil {
		t.Fatal("FailWithDetails should set Err")
	}
	if res.Err.Code != "dial" || res.Err.Category != CategoryConnection {
		t.Errorf("Err=%+v", res.Err)
	}
	if res.Err.Retryable {
		t.Error("Retryable should be false")
	}
	if got, _ := res.Err.Details["probedEndpoint"].(string); got != "http://x" {
		t.Errorf("Details[probedEndpoint]=%v", res.Err.Details["probedEndpoint"])
	}
}

func TestClassifyCDPErr_DeadlineExceeded(t *testing.T) {
	res := ClassifyCDPErr("op", context.DeadlineExceeded)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Code != "timeout" || res.Err.Category != CategoryTimeout || !res.Err.Retryable {
		t.Errorf("Err=%+v want timeout/timeout/retryable", res.Err)
	}
}

func TestClassifyCDPErr_Canceled(t *testing.T) {
	res := ClassifyCDPErr("op", context.Canceled)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Code != "canceled" || res.Err.Category != CategoryInternal {
		t.Errorf("Err=%+v want canceled/internal", res.Err)
	}
	if res.Err.Retryable {
		t.Error("canceled should not be retryable")
	}
}

func TestClassifyCDPErr_Closed(t *testing.T) {
	res := ClassifyCDPErr("op", cdp.ErrClosed)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != CategoryConnection || !res.Err.Retryable {
		t.Errorf("Err=%+v want connection/retryable", res.Err)
	}
	// Code stays the caller-supplied one when no override matched.
	if res.Err.Code != "op" {
		t.Errorf("code=%q want op (caller default preserved)", res.Err.Code)
	}
}

func TestClassifyCDPErr_Generic(t *testing.T) {
	res := ClassifyCDPErr("my_op", errors.New("something broke"))
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Code != "my_op" || res.Err.Category != CategoryProtocol {
		t.Errorf("Err=%+v want my_op/protocol", res.Err)
	}
	if res.Err.Retryable {
		t.Error("generic protocol error should not be retryable")
	}
	if res.Err.Details != nil {
		t.Errorf("generic error should not carry Details, got %v", res.Err.Details)
	}
}

func TestClassifyCDPErr_RemoteErrorWrapped(t *testing.T) {
	remote := &cdp.RemoteError{Code: -32000, Message: "boom", Data: "extra"}
	res := ClassifyCDPErr("cdp_call", remote)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	cdpErr, ok := res.Err.Details["cdpError"].(map[string]any)
	if !ok {
		t.Fatalf("Details[cdpError] is %T want map", res.Err.Details["cdpError"])
	}
	if cdpErr["code"] != -32000 {
		t.Errorf("cdpError.code=%v want -32000", cdpErr["code"])
	}
	if cdpErr["message"] != "boom" {
		t.Errorf("cdpError.message=%v want boom", cdpErr["message"])
	}
	if cdpErr["data"] != "extra" {
		t.Errorf("cdpError.data=%v want extra", cdpErr["data"])
	}
}

func TestClassifyCDPErr_RemoteErrorWrapped_Timeout(t *testing.T) {
	// A RemoteError joined with a deadline error: category/code follow the
	// deadline branch, and the cdpError details still attach.
	remote := &cdp.RemoteError{Code: -32001, Message: "slow"}
	joined := errors.Join(context.DeadlineExceeded, remote)
	res := ClassifyCDPErr("call", joined)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Code != "timeout" || res.Err.Category != CategoryTimeout {
		t.Errorf("Err=%+v want timeout/timeout", res.Err)
	}
	if _, ok := res.Err.Details["cdpError"]; !ok {
		t.Error("cdpError details should attach even on deadline branch")
	}
}

func TestReportProgress_NilSafe(t *testing.T) {
	// Nil receiver: must not panic.
	var e *RunEnv
	e.ReportProgress(1, 2, "x")

	// Non-nil with nil sink: no-op.
	env := &RunEnv{}
	env.ReportProgress(1, 2, "x")

	// Non-nil with sink: invoked with the supplied args.
	var gotCur, gotTotal float64
	var gotMsg string
	env.Progress = func(current, total float64, message string) {
		gotCur, gotTotal, gotMsg = current, total, message
	}
	env.ReportProgress(3, 10, "halfway")
	if gotCur != 3 || gotTotal != 10 || gotMsg != "halfway" {
		t.Errorf("progress got (%v,%v,%q)", gotCur, gotTotal, gotMsg)
	}
}

func TestLogf_NilSafe(t *testing.T) {
	var e *RunEnv
	e.Logf("info", "x %d", 1)

	env := &RunEnv{}
	env.Logf("info", "no sink %d", 1)

	var gotLevel, gotMsg string
	env.Log = func(level, message string) {
		gotLevel, gotMsg = level, message
	}
	env.Logf("warning", "count=%d name=%s", 7, "abc")
	if gotLevel != "warning" {
		t.Errorf("level=%q want warning", gotLevel)
	}
	if gotMsg != "count=7 name=abc" {
		t.Errorf("msg=%q", gotMsg)
	}
}

func TestBoolPtr(t *testing.T) {
	tp := BoolPtr(true)
	if tp == nil || *tp != true {
		t.Errorf("BoolPtr(true)=%v", tp)
	}
	fp := BoolPtr(false)
	if fp == nil || *fp != false {
		t.Errorf("BoolPtr(false)=%v", fp)
	}
}
