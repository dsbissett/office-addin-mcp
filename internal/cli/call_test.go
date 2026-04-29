package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunCallMissingTool(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall(nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("got exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--tool is required") {
		t.Errorf("missing usage hint, got %q", stderr.String())
	}
}

func TestRunCallUnknownTool(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{"--tool", "bogus.thing"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("got exit %d, want 1", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK {
		t.Fatal("expected ok=false")
	}
	if env.Error == nil || env.Error.Category != "not_found" {
		t.Fatalf("expected not_found error, got %+v", env.Error)
	}
}

func TestRunCallParamValidationFailsBeforeNetwork(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// Bad JSON for --param: should never touch the network, so a bogus
	// browser-url is fine — the call must fail at validation first.
	code := RunCall([]string{
		"--tool", "cdp.evaluate",
		"--param", "{not json",
		"--browser-url", "http://127.0.0.1:1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("got exit %d, want 1", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK {
		t.Fatal("expected ok=false")
	}
	if env.Error.Category != "validation" {
		t.Errorf("got category %q, want validation", env.Error.Category)
	}
}

func TestRunCallMissingExpression(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.evaluate",
		"--param", `{}`,
	}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("got exit %d, want 1", code)
	}
	var env Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "missing_expression" {
		t.Fatalf("expected missing_expression, got %+v", env.Error)
	}
}
