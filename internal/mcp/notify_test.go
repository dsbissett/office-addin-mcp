package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// newNotifyServer wires a server exposing a tool that emits three progress
// notifications and one log line, plus a client that records both. The client
// passes a progress token (without it the adapter installs no progress sink)
// and sets its logging level to info.
func newNotifyServer(t *testing.T) (*sdk.ClientSession, <-chan *sdk.ProgressNotificationParams, <-chan *sdk.LoggingMessageParams, func()) {
	t.Helper()

	reg := tools.NewRegistry()
	reg.MustRegister(tools.Tool{
		Name:        "fake.progress",
		Description: "emits progress + log",
		Schema:      json.RawMessage(`{"type":"object","additionalProperties":false}`),
		NoSession:   true,
		Run: func(_ context.Context, _ json.RawMessage, env *tools.RunEnv) tools.Result {
			env.Logf("info", "starting work")
			for i := 1; i <= 3; i++ {
				env.ReportProgress(float64(i), 3, "step")
			}
			return tools.OK(map[string]any{"done": true})
		},
	})

	mgr := session.NewManager(session.Config{})
	srv := NewServer(Options{Name: "test", Version: "v0", Registry: reg, Sessions: mgr})

	progressCh := make(chan *sdk.ProgressNotificationParams, 16)
	logCh := make(chan *sdk.LoggingMessageParams, 16)

	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.SDKServer().Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "client", Version: "v0"}, &sdk.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *sdk.ProgressNotificationClientRequest) {
			progressCh <- req.Params
		},
		LoggingMessageHandler: func(_ context.Context, req *sdk.LoggingMessageRequest) {
			logCh <- req.Params
		},
	})
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	if err := cs.SetLoggingLevel(ctx, &sdk.SetLoggingLevelParams{Level: "info"}); err != nil {
		t.Fatalf("set logging level: %v", err)
	}

	cleanup := func() {
		_ = cs.Close()
		_ = ss.Close()
		mgr.Close()
	}
	return cs, progressCh, logCh, cleanup
}

func TestCallToolStreamsProgressWhenTokenSupplied(t *testing.T) {
	cs, progressCh, logCh, cleanup := newNotifyServer(t)
	defer cleanup()

	params := &sdk.CallToolParams{Name: "fake.progress", Arguments: map[string]any{}}
	params.SetProgressToken("tok-1")

	res, err := cs.CallTool(context.Background(), params)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %+v", res)
	}

	got := collectProgress(t, progressCh, 3)
	if len(got) != 3 {
		t.Fatalf("got %d progress notifications, want 3", len(got))
	}
	for i, p := range got {
		if p.Progress != float64(i+1) {
			t.Errorf("progress[%d]=%v, want %d", i, p.Progress, i+1)
		}
		if p.Total != 3 {
			t.Errorf("progress[%d].Total=%v, want 3", i, p.Total)
		}
	}

	logs := collectLogs(t, logCh, 1)
	if len(logs) == 0 {
		t.Fatal("expected at least one log message")
	}
	if logs[0].Data != "starting work" {
		t.Errorf("log data=%v, want %q", logs[0].Data, "starting work")
	}
	if logs[0].Logger != "fake.progress" {
		t.Errorf("log logger=%q, want fake.progress", logs[0].Logger)
	}
}

func TestCallToolSuppressesProgressWithoutToken(t *testing.T) {
	cs, progressCh, _, cleanup := newNotifyServer(t)
	defer cleanup()

	// No progress token: the adapter must not install a progress sink, so the
	// tool's ReportProgress calls are silent.
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "fake.progress",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %+v", res)
	}
	select {
	case p := <-progressCh:
		t.Fatalf("received unexpected progress notification: %+v", p)
	case <-time.After(200 * time.Millisecond):
		// Expected: nothing arrived.
	}
}

func collectProgress(t *testing.T, ch <-chan *sdk.ProgressNotificationParams, want int) []*sdk.ProgressNotificationParams {
	t.Helper()
	var out []*sdk.ProgressNotificationParams
	deadline := time.After(2 * time.Second)
	for len(out) < want {
		select {
		case p := <-ch:
			out = append(out, p)
		case <-deadline:
			return out
		}
	}
	return out
}

func collectLogs(t *testing.T, ch <-chan *sdk.LoggingMessageParams, want int) []*sdk.LoggingMessageParams {
	t.Helper()
	var out []*sdk.LoggingMessageParams
	deadline := time.After(2 * time.Second)
	for len(out) < want {
		select {
		case p := <-ch:
			out = append(out, p)
		case <-deadline:
			return out
		}
	}
	return out
}
