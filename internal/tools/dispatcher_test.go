package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "update golden files in testdata/golden")

const fakeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "mode": {"type": "string", "enum": ["ok", "fail", "timeout"]}
  },
  "required": ["mode"],
  "additionalProperties": false
}`

func fakeTool() Tool {
	return Tool{
		Name:        "fake.run",
		Description: "Test-only tool that exercises dispatcher envelope shapes.",
		Schema:      json.RawMessage(fakeSchema),
		NoSession:   true,
		Run: func(ctx context.Context, raw json.RawMessage, _ *RunEnv) Result {
			var p struct {
				Mode string `json:"mode"`
			}
			_ = json.Unmarshal(raw, &p)
			switch p.Mode {
			case "ok":
				return OK(map[string]any{"answer": 42})
			case "fail":
				return Fail(CategoryProtocol, "fake_error", "synthetic failure", false)
			case "timeout":
				select {
				case <-time.After(5 * time.Second):
					return OK(nil)
				case <-ctx.Done():
					return ClassifyCDPErr("fake_op", ctx.Err())
				}
			}
			return Fail(CategoryInternal, "unknown_mode", p.Mode, false)
		},
	}
}

// canonicalize zeroes variable fields so envelopes can be diffed against
// golden files. Variable fields are: durationMs (timing), session/target/
// endpoint identifiers (server-assigned), error.message (library-specific
// wording).
func canonicalize(env Envelope) Envelope {
	env.Diagnostics.DurationMs = 0
	env.Diagnostics.SessionID = ""
	env.Diagnostics.TargetID = ""
	env.Diagnostics.Endpoint = ""
	if env.Error != nil {
		env.Error.Message = "<MESSAGE>"
	}
	return env
}

func assertGolden(t *testing.T, env Envelope, name string) {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(canonicalize(env)); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := buf.Bytes()

	path := filepath.Join("testdata", "golden", name+".json")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test -update` to create)", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("golden mismatch %s\n--- want\n%s--- got\n%s", path, want, got)
	}
}

func TestDispatch_Golden(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(fakeTool())

	cases := []struct {
		name       string
		tool       string
		params     string
		ctxTimeout time.Duration
	}{
		{"success", "fake.run", `{"mode":"ok"}`, 5 * time.Second},
		{"validation_error", "fake.run", `{}`, 5 * time.Second},
		{"cdp_error", "fake.run", `{"mode":"fail"}`, 5 * time.Second},
		{"timeout", "fake.run", `{"mode":"timeout"}`, 50 * time.Millisecond},
		{"unknown_tool", "missing.tool", `{}`, 5 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tc.ctxTimeout)
			defer cancel()
			env := Dispatch(ctx, reg, Request{Tool: tc.tool, Params: []byte(tc.params)})
			assertGolden(t, env, tc.name)
		})
	}
}

func TestDispatch_DiagnosticsAlwaysSet(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(fakeTool())

	env := Dispatch(context.Background(), reg, Request{
		Tool:   "fake.run",
		Params: []byte(`{"mode":"ok"}`),
	})
	if env.Diagnostics.Tool != "fake.run" {
		t.Errorf("tool=%q", env.Diagnostics.Tool)
	}
	if env.Diagnostics.EnvelopeVersion != EnvelopeVersion {
		t.Errorf("envelopeVersion=%q want %q",
			env.Diagnostics.EnvelopeVersion, EnvelopeVersion)
	}
	if env.Diagnostics.DurationMs < 0 {
		t.Errorf("negative durationMs: %d", env.Diagnostics.DurationMs)
	}
}
