package mcp

import (
	"context"
	"sort"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxCompletions caps the number of suggestions returned for one argument.
const maxCompletions = 100

// handleComplete answers completion/complete for prompt arguments. It completes
// the debug-tool-failure "tool" argument from the live tool registry and the
// draft-outlook-reply "tone" argument from a fixed vocabulary. Unknown
// references yield an empty (but valid) completion list, never an error.
func (s *Server) handleComplete(_ context.Context, req *sdk.CompleteRequest) (*sdk.CompleteResult, error) {
	values := s.completionCandidates(req.Params)
	return &sdk.CompleteResult{
		Completion: sdk.CompletionResultDetails{Values: values, Total: len(values)},
	}, nil
}

// completionCandidates resolves the candidate list for one completion request,
// already filtered by the partial value the user has typed.
func (s *Server) completionCandidates(p *sdk.CompleteParams) []string {
	if p == nil || p.Ref == nil || p.Ref.Type != "ref/prompt" {
		return nil
	}
	return filterByContains(s.promptArgValues(p.Ref.Name, p.Argument.Name), p.Argument.Value)
}

// promptArgValues returns the full candidate set for a (prompt, argument) pair.
func (s *Server) promptArgValues(prompt, arg string) []string {
	switch {
	case prompt == "debug-tool-failure" && arg == "tool":
		return s.toolNames()
	case prompt == "draft-outlook-reply" && arg == "tone":
		return replyTones()
	default:
		return nil
	}
}

// toolNames lists every registered tool name (already sorted by Registry.List).
func (s *Server) toolNames() []string {
	list := s.disp.Registry.List()
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, t.Name)
	}
	return out
}

// replyTones is the fixed vocabulary offered for draft-outlook-reply's tone.
func replyTones() []string {
	return []string{"professional", "formal", "casual", "friendly", "concise", "detailed", "apologetic", "enthusiastic"}
}

// filterByContains keeps candidates containing partial (case-insensitive),
// returns them sorted, and caps the result at maxCompletions.
func filterByContains(candidates []string, partial string) []string {
	needle := strings.ToLower(partial)
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if needle == "" || strings.Contains(strings.ToLower(c), needle) {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	if len(out) > maxCompletions {
		out = out[:maxCompletions]
	}
	return out
}
