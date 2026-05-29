package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// promptDef bundles a prompt's advertised definition with its handler.
type promptDef struct {
	prompt  *sdk.Prompt
	handler sdk.PromptHandler
}

// registerPrompts advertises the office-addin-mcp guided prompts. Prompts are
// reusable, parameterized workflows an MCP client can surface to the user
// (e.g. Claude Code's prompt/slash-command picker). They encode the recommended
// tool sequences for this server so a user can drive Office without knowing the
// individual tool names.
func registerPrompts(s *sdk.Server) {
	for _, p := range officePrompts() {
		s.AddPrompt(p.prompt, p.handler)
	}
}

// officePrompts is the canonical prompt set. Kept as a function (not a package
// var) so each call yields fresh pointers and tests can enumerate it.
func officePrompts() []promptDef {
	return []promptDef{
		{
			prompt: &sdk.Prompt{
				Name:        "debug-tool-failure",
				Title:       "Debug an Office tool failure",
				Description: "Diagnose and recover from a failed office-addin-mcp tool call using its structured error envelope.",
				Arguments: []*sdk.PromptArgument{
					{Name: "tool", Title: "Tool name", Description: "The tool that failed (e.g. excel.query).", Required: true},
					{Name: "error", Title: "Error text", Description: "The error message or JSON envelope returned.", Required: true},
				},
			},
			handler: handleDebugToolFailure,
		},
		{
			prompt: &sdk.Prompt{
				Name:        "connect-addin",
				Title:       "Connect to the Office add-in",
				Description: "Bring the Office add-in online and confirm a live CDP target before running other tools.",
			},
			handler: handleConnectAddin,
		},
		{
			prompt: &sdk.Prompt{
				Name:        "summarize-workbook",
				Title:       "Summarize the active workbook",
				Description: "Inspect the active Excel workbook and produce a concise structural summary.",
				Arguments: []*sdk.PromptArgument{
					{Name: "targetId", Title: "Target id", Description: "Optional exact CDP target id of the add-in taskpane."},
				},
			},
			handler: handleSummarizeWorkbook,
		},
		{
			prompt: &sdk.Prompt{
				Name:        "draft-outlook-reply",
				Title:       "Draft an Outlook reply",
				Description: "Draft a reply to the open Outlook message following your instructions.",
				Arguments: []*sdk.PromptArgument{
					{Name: "instructions", Title: "Instructions", Description: "What the reply should say.", Required: true},
					{Name: "tone", Title: "Tone", Description: "Optional tone (e.g. professional, friendly, concise)."},
				},
			},
			handler: handleDraftOutlookReply,
		},
		{
			prompt: &sdk.Prompt{
				Name:        "rebuild-slide",
				Title:       "Rebuild a slide from an outline",
				Description: "Rebuild the active PowerPoint slide from a title-and-bullets outline.",
				Arguments: []*sdk.PromptArgument{
					{Name: "outline", Title: "Outline", Description: "The slide outline (title plus bullet points).", Required: true},
				},
			},
			handler: handleRebuildSlide,
		},
	}
}

func handleDebugToolFailure(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
	a := req.Params.Arguments
	text := fmt.Sprintf(debugFailureTemplate, argOr(a, "tool", "the tool"), argOr(a, "error", "(no error text provided)"))
	return userPrompt("Diagnose and recover from an office-addin-mcp tool failure", text), nil
}

func handleConnectAddin(_ context.Context, _ *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
	return userPrompt("Connect to the Office add-in", connectAddinTemplate), nil
}

func handleSummarizeWorkbook(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
	text := fmt.Sprintf(summarizeWorkbookTemplate, targetClause(argOr(req.Params.Arguments, "targetId", "")))
	return userPrompt("Summarize the active Excel workbook", text), nil
}

func handleDraftOutlookReply(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
	a := req.Params.Arguments
	text := fmt.Sprintf(draftReplyTemplate, argOr(a, "tone", "professional"), argOr(a, "instructions", "(no instructions provided)"))
	return userPrompt("Draft an Outlook reply", text), nil
}

func handleRebuildSlide(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
	text := fmt.Sprintf(rebuildSlideTemplate, argOr(req.Params.Arguments, "outline", "(no outline provided)"))
	return userPrompt("Rebuild a PowerPoint slide from an outline", text), nil
}

// userPrompt builds a single-message GetPromptResult spoken in the user role.
func userPrompt(desc, text string) *sdk.GetPromptResult {
	return &sdk.GetPromptResult{
		Description: desc,
		Messages: []*sdk.PromptMessage{
			{Role: sdk.Role("user"), Content: &sdk.TextContent{Text: text}},
		},
	}
}

// argOr returns args[key] when present and non-empty, else fallback.
func argOr(args map[string]string, key, fallback string) string {
	if v, ok := args[key]; ok && v != "" {
		return v
	}
	return fallback
}

// targetClause renders the optional targetId into a sentence fragment.
func targetClause(targetID string) string {
	if targetID == "" {
		return "the active add-in taskpane"
	}
	return "target id " + targetID
}

const debugFailureTemplate = `A call to the office-addin-mcp tool %q failed with:

%s

Recover from it methodically:
1. Read the failure envelope's "error" object — note "category", "code", "retryable", and "recoveryHint".
2. If "details.recoverableViaTool" names a tool (often addin.ensureRunning), call that tool first, then retry.
3. Use the structured "details" instead of parsing the English message: for Excel inspect "available_sheets", "nearest_name_suggestions", and "parsed_address"; for Outlook "item_mode"; for PowerPoint "slide_count".
4. Fix the offending parameters and retry the call exactly once. If it still fails, report the category and recoveryHint to the user rather than looping.`

const connectAddinTemplate = `Bring the Office add-in online and confirm it is reachable before doing anything else:
1. Call addin.ensureRunning to start (or reuse) the add-in and its WebView2 debug endpoint.
2. Call addin.status and confirm "reachable" is true; if not, follow its recoveryHints.
3. Call pages.list to confirm a taskpane target is present, and note its targetId for later tool calls.
Report the connected target, then proceed with the user's request.`

const summarizeWorkbookTemplate = `Summarize the active Excel workbook (%s):
1. Call excel.discover to learn the workbook's sheets and shape.
2. Call excel.summarizeWorkbook for sheet dimensions, used ranges, and tables.
3. Produce a concise summary: worksheet names, used-range sizes, notable tables/charts, and anything that looks like a data region worth analyzing.`

const draftReplyTemplate = `Draft a %s reply to the currently open Outlook message:
1. Follow these instructions for the content: %s
2. Call outlook.draftReply with the composed body. Do not send — only draft.
3. Show the drafted reply to the user for confirmation.`

const rebuildSlideTemplate = `Rebuild the active PowerPoint slide from this outline:

%s

Call powerpoint.rebuildSlideFromOutline with the title and bullet points parsed from the outline, then confirm the slide was rebuilt.`
