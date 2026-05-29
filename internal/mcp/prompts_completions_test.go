package mcp

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverForTest builds a Server wired to the full default registry (so tool-name
// completion has realistic data) without connecting any transport.
func serverForTest(t *testing.T) *Server {
	t.Helper()
	return NewServer(Options{Name: "t", Version: "v0", Registry: DefaultRegistry(), DisableAutoRecover: true})
}

func promptText(t *testing.T, res *sdk.GetPromptResult) string {
	t.Helper()
	if res == nil || len(res.Messages) == 0 {
		t.Fatal("prompt result has no messages")
	}
	tc, ok := res.Messages[0].Content.(*sdk.TextContent)
	if !ok {
		t.Fatalf("message content type %T, want *TextContent", res.Messages[0].Content)
	}
	if res.Messages[0].Role != sdk.Role("user") {
		t.Errorf("role = %q, want user", res.Messages[0].Role)
	}
	return tc.Text
}

func TestOfficePrompts_WellFormed(t *testing.T) {
	prompts := officePrompts()
	if len(prompts) != 5 {
		t.Fatalf("got %d prompts, want 5", len(prompts))
	}
	seen := map[string]bool{}
	for _, p := range prompts {
		if p.prompt == nil || p.handler == nil {
			t.Fatalf("prompt %+v missing definition or handler", p.prompt)
		}
		if p.prompt.Name == "" || p.prompt.Title == "" || p.prompt.Description == "" {
			t.Errorf("prompt %q missing name/title/description", p.prompt.Name)
		}
		if seen[p.prompt.Name] {
			t.Errorf("duplicate prompt name %q", p.prompt.Name)
		}
		seen[p.prompt.Name] = true
		// Every handler must return a usable result for empty args.
		res, err := p.handler(context.Background(), &sdk.GetPromptRequest{Params: &sdk.GetPromptParams{Name: p.prompt.Name}})
		if err != nil {
			t.Errorf("handler %q error: %v", p.prompt.Name, err)
			continue
		}
		if promptText(t, res) == "" {
			t.Errorf("handler %q produced empty text", p.prompt.Name)
		}
	}
}

func TestPromptHandlers_InterpolateArgs(t *testing.T) {
	cases := []struct {
		name string
		args map[string]string
		want []string
	}{
		{"debug-tool-failure", map[string]string{"tool": "excel.query", "error": "ItemNotFound boom"}, []string{"excel.query", "ItemNotFound boom", "recoveryHint"}},
		{"summarize-workbook", map[string]string{"targetId": "T-123"}, []string{"target id T-123", "excel.summarizeWorkbook"}},
		{"summarize-workbook", nil, []string{"the active add-in taskpane"}},
		{"draft-outlook-reply", map[string]string{"instructions": "say yes", "tone": "friendly"}, []string{"friendly", "say yes", "outlook.draftReply"}},
		{"draft-outlook-reply", map[string]string{"instructions": "say yes"}, []string{"professional"}},
		{"rebuild-slide", map[string]string{"outline": "Title\n- a\n- b"}, []string{"Title", "powerpoint.rebuildSlideFromOutline"}},
		{"connect-addin", nil, []string{"addin.ensureRunning", "addin.status", "pages.list"}},
	}
	handlers := map[string]sdk.PromptHandler{}
	for _, p := range officePrompts() {
		handlers[p.prompt.Name] = p.handler
	}
	for _, c := range cases {
		h, ok := handlers[c.name]
		if !ok {
			t.Fatalf("no handler for %q", c.name)
		}
		res, err := h(context.Background(), &sdk.GetPromptRequest{Params: &sdk.GetPromptParams{Name: c.name, Arguments: c.args}})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		text := promptText(t, res)
		for _, want := range c.want {
			if !strings.Contains(text, want) {
				t.Errorf("%s text missing %q:\n%s", c.name, want, text)
			}
		}
	}
}

func TestCompletion_ToolNames(t *testing.T) {
	s := serverForTest(t)
	res, err := s.handleComplete(context.Background(), &sdk.CompleteRequest{Params: &sdk.CompleteParams{
		Ref:      &sdk.CompleteReference{Type: "ref/prompt", Name: "debug-tool-failure"},
		Argument: sdk.CompleteParamsArgument{Name: "tool", Value: "excel"},
	}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(res.Completion.Values) == 0 {
		t.Fatal("no completions for tool=excel")
	}
	for _, v := range res.Completion.Values {
		if !strings.Contains(strings.ToLower(v), "excel") {
			t.Errorf("value %q does not contain 'excel'", v)
		}
	}
	if res.Completion.Total != len(res.Completion.Values) {
		t.Errorf("Total=%d, want %d", res.Completion.Total, len(res.Completion.Values))
	}
}

func TestCompletion_ToneVocabulary(t *testing.T) {
	s := serverForTest(t)
	res, err := s.handleComplete(context.Background(), &sdk.CompleteRequest{Params: &sdk.CompleteParams{
		Ref:      &sdk.CompleteReference{Type: "ref/prompt", Name: "draft-outlook-reply"},
		Argument: sdk.CompleteParamsArgument{Name: "tone", Value: "con"},
	}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(res.Completion.Values) != 1 || res.Completion.Values[0] != "concise" {
		t.Errorf("tone 'con' completions = %v, want [concise]", res.Completion.Values)
	}
}

func TestCompletion_UnknownAndNilSafe(t *testing.T) {
	s := serverForTest(t)
	cases := []*sdk.CompleteParams{
		nil,
		{Ref: nil, Argument: sdk.CompleteParamsArgument{Name: "x"}},
		{Ref: &sdk.CompleteReference{Type: "ref/resource", URI: "office://excel/foo"}, Argument: sdk.CompleteParamsArgument{Name: "x"}},
		{Ref: &sdk.CompleteReference{Type: "ref/prompt", Name: "debug-tool-failure"}, Argument: sdk.CompleteParamsArgument{Name: "error", Value: "z"}},
		{Ref: &sdk.CompleteReference{Type: "ref/prompt", Name: "no-such-prompt"}, Argument: sdk.CompleteParamsArgument{Name: "tool"}},
	}
	for i, p := range cases {
		got := s.completionCandidates(p)
		if len(got) != 0 {
			t.Errorf("case %d: expected no candidates, got %v", i, got)
		}
	}
}

func TestFilterByContains_SortedAndCapped(t *testing.T) {
	in := make([]string, 0, 150)
	for i := 0; i < 150; i++ {
		// zero-padded so lexical sort is well-defined
		in = append(in, "tool-"+string(rune('a'+i%26))+padNum(i))
	}
	got := filterByContains(in, "tool")
	if len(got) != maxCompletions {
		t.Fatalf("len=%d, want cap %d", len(got), maxCompletions)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("not sorted at %d: %q > %q", i, got[i-1], got[i])
		}
	}
	if v := filterByContains(in, "NOPE"); len(v) != 0 {
		t.Errorf("expected no matches, got %d", len(v))
	}
}

func padNum(i int) string {
	s := ""
	for _, d := range []int{i / 100, (i / 10) % 10, i % 10} {
		s += string(rune('0' + d))
	}
	return s
}

// TestPromptsAndCompletionsOverTransport proves registerPrompts and the
// CompletionHandler are wired end-to-end through the SDK.
func TestPromptsAndCompletionsOverTransport(t *testing.T) {
	cs, cleanup := newTestServer(t)
	defer cleanup()
	ctx := context.Background()

	list, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	names := map[string]bool{}
	for _, p := range list.Prompts {
		names[p.Name] = true
	}
	for _, want := range []string{"debug-tool-failure", "connect-addin", "summarize-workbook", "draft-outlook-reply", "rebuild-slide"} {
		if !names[want] {
			t.Errorf("ListPrompts missing %q", want)
		}
	}

	gp, err := cs.GetPrompt(ctx, &sdk.GetPromptParams{Name: "debug-tool-failure", Arguments: map[string]string{"tool": "excel.query", "error": "boom"}})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if txt := promptText(t, gp); !strings.Contains(txt, "excel.query") {
		t.Errorf("GetPrompt text missing tool name: %s", txt)
	}

	comp, err := cs.Complete(ctx, &sdk.CompleteParams{
		Ref:      &sdk.CompleteReference{Type: "ref/prompt", Name: "debug-tool-failure"},
		Argument: sdk.CompleteParamsArgument{Name: "tool", Value: "fake"},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	found := false
	for _, v := range comp.Completion.Values {
		if v == "fake.run" {
			found = true
		}
	}
	if !found {
		t.Errorf("Complete values %v missing fake.run", comp.Completion.Values)
	}
}
