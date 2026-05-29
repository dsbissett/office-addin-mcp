package cdptest_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/officejs"
)

func TestServer_EvalRoundTrip(t *testing.T) {
	srv := cdptest.NewServer(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method != "Runtime.evaluate" {
			return map[string]any{}, nil
		}
		return cdptest.EvalOffice(map[string]any{"address": "A1:B2"}), nil
	})
	conn := srv.Dial(t)

	res, err := conn.Evaluate(context.Background(), "cdp-1", cdp.EvaluateParams{Expression: "1", ReturnByValue: true})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Result == nil {
		t.Fatal("nil result")
	}
	var env struct {
		Result struct {
			Address string `json:"address"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res.Result.Value, &env); err != nil {
		t.Fatalf("decode value: %v", err)
	}
	if env.Result.Address != "A1:B2" {
		t.Errorf("address=%q, want A1:B2", env.Result.Address)
	}
}

// TestServer_ExecutorHappyPath proves the harness drives the real officejs
// executor — the exact path every host tool's RunPayload takes.
func TestServer_ExecutorHappyPath(t *testing.T) {
	srv := cdptest.NewServer(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice(map[string]any{"answer": 42}), nil
	})
	exec := officejs.New(srv.Dial(t), "cdp-1")

	raw, err := exec.Run(context.Background(), "excel.readRange", map[string]any{"address": "A1"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var got struct {
		Answer int `json:"answer"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Answer != 42 {
		t.Errorf("answer=%d, want 42", got.Answer)
	}
}

func TestServer_ExecutorOfficeError(t *testing.T) {
	srv := cdptest.NewServer(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOfficeErr("ItemNotFound", "Worksheet not found", nil), nil
	})
	exec := officejs.New(srv.Dial(t), "cdp-1")

	_, err := exec.Run(context.Background(), "excel.readRange", map[string]any{"address": "A1"})
	var oerr *officejs.OfficeError
	if !errors.As(err, &oerr) {
		t.Fatalf("want *officejs.OfficeError, got %T: %v", err, err)
	}
	if oerr.Code != "ItemNotFound" {
		t.Errorf("code=%q", oerr.Code)
	}
}

func TestServer_RemoteError(t *testing.T) {
	srv := cdptest.NewServer(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return nil, &cdp.RemoteError{Code: -32601, Message: "Method not found"}
	})
	conn := srv.Dial(t)

	_, err := conn.Send(context.Background(), "", "Bogus.method", nil)
	var rerr *cdp.RemoteError
	if !errors.As(err, &rerr) {
		t.Fatalf("want *cdp.RemoteError, got %T: %v", err, err)
	}
	if rerr.Code != -32601 {
		t.Errorf("code=%d", rerr.Code)
	}
}
