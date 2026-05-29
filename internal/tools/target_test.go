package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
)

// targetsResponder builds a cdptest.Responder that returns the supplied target
// list for Target.getTargets, a fixed sessionId for Target.attachToTarget, and
// a fresh targetId for Target.createTarget. Any other method returns an empty
// object.
func targetsResponder(t *testing.T, targets []cdp.TargetInfo) cdptest.Responder {
	t.Helper()
	return func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "Target.getTargets":
			return map[string]any{"targetInfos": targets}, nil
		case "Target.attachToTarget":
			return map[string]any{"sessionId": "cdp-sess-1"}, nil
		case "Target.createTarget":
			return map[string]any{"targetId": "created-1"}, nil
		default:
			return map[string]any{}, nil
		}
	}
}

func TestResolveTarget_ByTargetID(t *testing.T) {
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "a", Type: "page", URL: "https://addin/taskpane.html"},
		{TargetID: "b", Type: "page", URL: "https://other"},
	}))
	conn := srv.Dial(t)

	got, err := ResolveTarget(context.Background(), conn, TargetSelector{TargetID: "b"}, nil)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if got.TargetID != "b" {
		t.Errorf("targetId=%q want b", got.TargetID)
	}
}

func TestResolveTarget_ByTargetID_NotFound(t *testing.T) {
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "a", Type: "page", URL: "https://addin"},
	}))
	conn := srv.Dial(t)

	_, err := ResolveTarget(context.Background(), conn, TargetSelector{TargetID: "missing"}, nil)
	if err == nil || !strings.Contains(err.Error(), "no target with targetId") {
		t.Fatalf("err=%v want no-target-with-targetId", err)
	}
}

func TestResolveTarget_ByURLPattern(t *testing.T) {
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "a", Type: "page", URL: "https://example/taskpane.html"},
		{TargetID: "b", Type: "page", URL: "https://example/dialog.html"},
	}))
	conn := srv.Dial(t)

	got, err := ResolveTarget(context.Background(), conn, TargetSelector{URLPattern: "dialog"}, nil)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if got.TargetID != "b" {
		t.Errorf("targetId=%q want b", got.TargetID)
	}
}

func TestResolveTarget_ByURLPattern_NotFound(t *testing.T) {
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "a", Type: "page", URL: "https://example/taskpane.html"},
	}))
	conn := srv.Dial(t)

	_, err := ResolveTarget(context.Background(), conn, TargetSelector{URLPattern: "nope"}, nil)
	if err == nil || !strings.Contains(err.Error(), "no target with url containing") {
		t.Fatalf("err=%v want no-target-with-url", err)
	}
}

func TestResolveTarget_BySurface_Heuristic(t *testing.T) {
	// No manifest: ClassifyTargets falls back to URL heuristics. A plain https
	// page is classified as a taskpane, so a taskpane surface request matches.
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "a", Type: "page", URL: "https://example/page.html"},
	}))
	conn := srv.Dial(t)

	got, err := ResolveTarget(context.Background(), conn, TargetSelector{Surface: addin.SurfaceTaskpane}, nil)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if got.TargetID != "a" {
		t.Errorf("targetId=%q want a", got.TargetID)
	}
}

func TestResolveTarget_BySurface_HeuristicMiss(t *testing.T) {
	// The heuristic never emits SurfaceContent, so requesting it against a plain
	// https page (classified taskpane) misses.
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "a", Type: "page", URL: "https://example/page.html"},
	}))
	conn := srv.Dial(t)

	_, err := ResolveTarget(context.Background(), conn, TargetSelector{Surface: addin.SurfaceContent}, nil)
	if err == nil || !strings.Contains(err.Error(), "no target classified as surface") {
		t.Fatalf("err=%v want no-target-classified", err)
	}
}

func TestResolveTarget_BySurface_ManifestMatch(t *testing.T) {
	manifest := &addin.Manifest{
		ID: "addin-123",
		Surfaces: []addin.Surface{
			{Type: addin.SurfaceTaskpane, URL: "https://example/taskpane.html", Pattern: "taskpane.html"},
		},
	}
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "tp", Type: "page", URL: "https://example/taskpane.html"},
		{TargetID: "x", Type: "page", URL: "https://example/other.html"},
	}))
	conn := srv.Dial(t)

	got, err := ResolveTarget(context.Background(), conn,
		TargetSelector{Surface: addin.SurfaceTaskpane}, manifest)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if got.TargetID != "tp" {
		t.Errorf("targetId=%q want tp", got.TargetID)
	}
}

func TestResolveTarget_BySurface_AddinIDMismatch(t *testing.T) {
	// Surface matches but the requested AddinID does not match the loaded
	// manifest's ID, so the classified target is skipped and we miss.
	manifest := &addin.Manifest{
		ID: "addin-123",
		Surfaces: []addin.Surface{
			{Type: addin.SurfaceTaskpane, URL: "https://example/taskpane.html", Pattern: "taskpane.html"},
		},
	}
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "tp", Type: "page", URL: "https://example/taskpane.html"},
	}))
	conn := srv.Dial(t)

	_, err := ResolveTarget(context.Background(), conn,
		TargetSelector{Surface: addin.SurfaceTaskpane, AddinID: "other-id"}, manifest)
	if err == nil || !strings.Contains(err.Error(), "no target classified as surface") {
		t.Fatalf("err=%v want no-target-classified", err)
	}
}

func TestResolveTarget_BySurface_AddinIDMatch(t *testing.T) {
	manifest := &addin.Manifest{
		ID: "addin-123",
		Surfaces: []addin.Surface{
			{Type: addin.SurfaceTaskpane, URL: "https://example/taskpane.html", Pattern: "taskpane.html"},
		},
	}
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "tp", Type: "page", URL: "https://example/taskpane.html"},
	}))
	conn := srv.Dial(t)

	got, err := ResolveTarget(context.Background(), conn,
		TargetSelector{Surface: addin.SurfaceTaskpane, AddinID: "addin-123"}, manifest)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if got.TargetID != "tp" {
		t.Errorf("targetId=%q want tp", got.TargetID)
	}
}

func TestResolveTarget_DefaultPickFirstPage(t *testing.T) {
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "dev", Type: "page", URL: "devtools://devtools/bundled"},
		{TargetID: "real", Type: "page", URL: "https://example/page.html"},
	}))
	conn := srv.Dial(t)

	got, err := ResolveTarget(context.Background(), conn, TargetSelector{}, nil)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if got.TargetID != "real" {
		t.Errorf("targetId=%q want real (devtools page skipped)", got.TargetID)
	}
}

func TestResolveTarget_DefaultCreatesAboutBlank(t *testing.T) {
	// No page targets at all: ResolveTarget falls through to Target.createTarget.
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "bg", Type: "background_page", URL: "https://ext/bg"},
	}))
	conn := srv.Dial(t)

	got, err := ResolveTarget(context.Background(), conn, TargetSelector{}, nil)
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if got.TargetID != "created-1" {
		t.Errorf("targetId=%q want created-1", got.TargetID)
	}
	if got.URL != "about:blank" || got.Type != "page" {
		t.Errorf("created target=%+v want type=page url=about:blank", got)
	}
}

func TestResolveTarget_GetTargetsError(t *testing.T) {
	srv := cdptest.NewServer(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Target.getTargets" {
			return nil, &cdp.RemoteError{Code: -32000, Message: "boom"}
		}
		return map[string]any{}, nil
	})
	conn := srv.Dial(t)

	_, err := ResolveTarget(context.Background(), conn, TargetSelector{}, nil)
	if err == nil {
		t.Fatal("expected error from getTargets failure")
	}
}

func TestResolveTarget_CreateTargetError(t *testing.T) {
	// No page targets, and createTarget fails — exercises the wrapped error.
	srv := cdptest.NewServer(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "Target.getTargets":
			return map[string]any{"targetInfos": []cdp.TargetInfo{}}, nil
		case "Target.createTarget":
			return nil, &cdp.RemoteError{Code: -32000, Message: "cannot create"}
		default:
			return map[string]any{}, nil
		}
	})
	conn := srv.Dial(t)

	_, err := ResolveTarget(context.Background(), conn, TargetSelector{}, nil)
	if err == nil || !strings.Contains(err.Error(), "createTarget failed") {
		t.Fatalf("err=%v want createTarget-failed", err)
	}
}

func TestIsInternalURL(t *testing.T) {
	cases := map[string]bool{
		"devtools://devtools/bundled":       true,
		"chrome://version":                  true,
		"edge://settings":                   true,
		"https://example.com/taskpane.html": false,
		"about:blank":                       false,
		"":                                  false,
	}
	for u, want := range cases {
		if got := IsInternalURL(u); got != want {
			t.Errorf("IsInternalURL(%q)=%v want %v", u, got, want)
		}
	}
}
