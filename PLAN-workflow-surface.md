# office-addin-mcp: workflow surface, query engine, resources, cache, diagnostics, macros

## Context

The current MCP server exposes ~411 raw `cdp.*` methods (gated behind `--expose-raw-cdp`) plus a host surface dominated by single-object getters/setters (`excel.readRange`, `word.readBody`, `outlook.getSubject`, …). That shape is a CDP/Office.js SDK, not an agent surface — LLMs perform poorly when forced to compose 20 primitive calls and reason over 100k cells in tokens.

This plan does two things:

1. **Narrows the surface.** Delete the `cdptool` package and the generated CDP tools (~10.5K LOC). Stop registering host *primitive* tools as MCP tools. Keep the underlying JS payloads (`internal/js/*.js`) — they become reusable building blocks for new workflow tools, not directly-exposed verbs. Keep the `*.runScript` escape hatches and the `page.*` / `inspect` / `interact` tools (already workflow-shaped — the only path into the task-pane UI).

2. **Adds 7 capabilities** in phases: workflow tools, server-side query engine, cross-host orchestration, MCP Resources + subscriptions, persistent document context cache, auto-diagnostics enrichment, and record/replay macros.

Outcome: a small, opinionated, agent-shaped surface (~25–35 tools) that covers the multiplicative value of multi-host Office automation while the old SDK-shaped surface either disappears or moves behind `*.runScript`.

## Architecture findings (anchors)

- **Executor**: `internal/officejs/executor.go:56` — `Executor.Run(ctx, toolName, args)` evaluates an embedded JS payload via CDP `Runtime.evaluate` inside `Excel.run` / `Word.run` / etc. Returns `{result, __officeError, code, message, debugInfo}`. Reusable as-is for composing new workflow payloads.
- **Embedded payloads**: `internal/js/embed.go:13` (`go:embed all:*.js`) + `internal/js/payloads.go:134` (filename → tool-name mapping). 52 payloads today; we will add new ones, not reshape the loader.
- **Tool type**: `internal/tools/registry.go:15` — `tools.Tool{Name, Description, Schema, OutputSchema, Annotations, Run, NoSession}`. `runPayloadSum(...)` (in each `*tool` package) is the canonical wrapper for a tool that calls one payload + summarizes the result.
- **Registry composition**: `internal/mcp/registry.go:33` — `DefaultRegistry(CDPSelection)`. Single chokepoint for the tool surface.
- **Dispatcher**: `internal/tools/dispatcher.go:70` — `Dispatch()` is the single resolve→validate→acquire→run→finalize path. Pre/post hooks for record/replay and diagnostics enrichment must wrap `tool.Run(...)` here (line 145) and inside `finalize()` (line 150).
- **Envelope + Diagnostics**: `internal/tools/result.go:37` — typed `Diagnostics` (Tool, EnvelopeVersion, RequestID, SessionID, CDPSessionID, TargetID, Endpoint, CDPRoundTrips, DurationMs). `EnvelopeError.Details map[string]any` (line ~69) is where recovery hints land. Existing pattern: `classifyAcquireErr` in dispatcher.go:162. We add a sibling `classifyOfficeJSErr`.
- **MCP transport**: `internal/mcp/server.go:124` — official `go-sdk/mcp` over `StdioTransport`. Tools-only today. SDK supports `AddResource`/notifications; nothing wired yet.
- **Session manager**: `internal/session/` — owns CDP connection, selector cache, snapshot, default selection. Per-session arbitrary state attaches via the `Snapshot` struct (`RunEnv.Snapshot()` / `RunEnv.SetSnapshot()` already exist — natural home for the document cache).
- **Outlook quirk**: no `Outlook.run`; `__runOutlook(fn => fn(Office.context.mailbox))` (preamble in `internal/js/_preamble.js`). Workflow tools that target Outlook must follow this pattern.

## Phase 0 — Surface narrowing (foundation)

Goal: ship a build with the new minimum surface before adding workflows.

1. **Delete the CDP tool surface**
   - Remove `internal/tools/cdptool/` (hand-written `register.go`, generated `generated/*.go`, ~10.5K LOC).
   - Remove `cmd/gen-cdp-tools/` generator and its CDP protocol JSON.
   - Remove `--expose-raw-cdp` flag and `OFFICE_ADDIN_MCP_EXPOSE_RAW_CDP` env (`cmd/office-addin-mcp/main.go:51,99,178`); remove `CDPSelection` plumbing from `internal/mcp/registry.go:24-51`.
   - Update `cmd/office-addin-mcp/main.go` to call `DefaultRegistry()` (no args).
   - Update `CLAUDE.md`: drop the "411 raw cdp.* methods" sentence; describe the new surface.
   - Update README "Tool Groups" table.

2. **Unregister host primitive tools (do not delete files yet)**
   - In each of `internal/tools/{exceltool,wordtool,outlooktool,powerpointtool,onenotetool}/register.go`, comment out / remove the `r.MustRegister(...)` calls for primitives. Keep the `RunScript()` registration.
   - Keep the Go files (range.go, body.go, presentation.go, …) compiling — their `runPayloadSum` calls are the template for workflow tools, and tests still cover them.
   - Tag the unregistered tool functions with `// internal: still callable from Go workflows; not exposed as MCP tool` so future cleanup is obvious.

3. **Verify**
   - `go build ./...` and `go test ./...` pass.
   - `golangci-lint run` passes.
   - `office-addin-mcp list-tools` (or equivalent) shows: addin.*, page.*, pages.*, inspect.*, interact.*, plus `excel.runScript`, `word.runScript`, `outlook.runScript`, `powerpoint.runScript`, `onenote.runScript`, plus `addin.detect/launch/stop`. No `cdp.*`. No `excel.readRange` etc.
   - Update golden fixtures in `internal/tools/testdata/golden/` if any reference removed tools.

## Phase A — Workflow tools (Feature 1) + cross-host orchestration (Feature 3)

Goal: the agent-shaped verbs the surface should have always had.

**New host package(s)**: keep the existing `*tool` package layout. Add a new top-level package `internal/tools/officetool/` for cross-host workflows.

**Workflow tools to ship (initial cut — bias to high-value, defer the long tail):**

Excel
- `excel.tabulate_region` — load a range, infer header row, return a typed table (rows + column types). Composes existing `excel_read_range.js`.
- `excel.apply_diff` — input: list of `{address, value, formula?}` patches. Server batches into one `Excel.run` (single CDP round-trip). New payload `excel_apply_diff.js`.
- `excel.summarize_workbook` — one-call discovery: sheet list, table list, named ranges, used-range bounds per sheet. Composes existing list/info payloads server-side.

Word
- `word.restructure_outline` — input: outline tree (heading → children). Replaces document body with the new structure preserving styles. New payload.
- `word.apply_edits` — input: `{find, replace}[]` or range-based patches; one `Word.run` batch.

Outlook
- `outlook.triage_inbox` — input: list of rule predicates (subject pattern, sender, date range) + actions (`mark_read`, `move`, `flag`). Loops over `mailbox.getCallbackUrlAsync` / EWS-equivalent in one payload.
- `outlook.draft_reply` — input: `{tone, key_points[]}`; sets body + subject in compose mode in one call.

PowerPoint
- `powerpoint.rebuild_slide_from_outline` — input: slide outline (title, bullets, layout); rebuilds slide N in one `PowerPoint.run`.

OneNote
- `onenote.append_to_page` — append HTML/outline to a page in one call.

Cross-host (`internal/tools/officetool/`)
- `office.embed` — `source: excel:Sheet1!A1:D20` / `target: pp:slide3` (or `word:bookmark:foo`). Reads source range, writes as a table/picture into target host. Two CDP sessions, sequenced in Go. Drives the multi-host story (F7–F9) into actual multiplicative value.
- `office.fill_template` — `template: word_doc, data_source: excel_range`. Reads the range as records, runs find/replace on the doc copy.
- `office.export` — `source: word|powerpoint, format: pdf` via host `getFileAsync` / `convertToPdf`.

**Implementation pattern (per workflow tool):**
1. Author the JS payload in `internal/js/<host>_<verb>.js`.
2. Add `<verb>.go` in `internal/tools/<host>tool/` defining `Tool()` returning `tools.Tool{Name, Schema, Run}`.
3. Register in `internal/tools/<host>tool/register.go`.
4. Add a unit test that exercises the schema + summary; live Excel verification is manual.

**Cross-host implementation notes:**
- `officetool.embed` etc. need to run two payloads against potentially different targets in one tool call. `RunEnv.Attach` + `executor.Run` can be called sequentially; no infrastructure change needed. The Go workflow stitches results.
- Source/target URI grammar: `excel:<sheet>!<range>`, `pp:slide<N>`, `word:bookmark:<name>`, `outlook:item`, `onenote:section/<name>/page/<name>`. Same grammar feeds Phase D resources.

## Phase B — Server-side query engine (Feature 2) + persistent context cache (Feature 5)

Goal: 100k-row workbook becomes a 5-row answer; subsequent sessions skip re-discovery.

**Feature 2: `excel.query` (and host equivalents)**
- New payload `excel_query.js`: load a range's `values`, then evaluate a tiny expression engine in JS (filter / project / groupby / agg). Bias to a JSONLogic-shaped DSL — JSON in, JSON out, easy to schema-validate. Avoid SQL parsing in JS.
- Schema:
  ```json
  { "range": "Sheet1!A1:F2000",
    "headers": "first_row" | "none" | ["explicit", "names"],
    "filter": <jsonlogic>,
    "project": ["col1","col2"],
    "groupBy": ["sku"], "agg": [{"col":"qty","fn":"sum"}],
    "limit": 100 }
  ```
- Same shape for `outlook.query` (over a folder), `onenote.query` (over pages), `powerpoint.query` (over slide shape filters). Each gets its own payload that loads the right collection, then reuses a shared JS query helper extracted to `internal/js/_query.js` (preamble-included).

**Feature 5: persistent document context cache**
- New package `internal/doccache/` with `Store` keyed on `(filePath, hostFingerprint)`. Backing file: `%LOCALAPPDATA%\office-addin-mcp\doccache.json` (Windows) / `$XDG_CACHE_HOME/...` elsewhere, mode 0600 — same convention as `daemon.json`.
- Cached payload (per workbook): sheet list, used-range bounds, table catalog, named ranges, last-seen ETag-equivalent (workbook `getProtection` + sheet count + sum of used-range cell counts as a fingerprint — Office.js doesn't expose true ETags).
- New tool `excel.discover` — first call populates the cache; subsequent calls within a session return the cached snapshot in one tool turn. Identical wrapper for the other hosts.
- Wire into existing `RunEnv.Snapshot()` for in-session reuse; persist to disk on session close (hook in `internal/session/Manager.Drop` or equivalent).
- Pairs naturally with the selector cache — same pattern (cache key + invalidate on miss).

**Tradeoff to flag during execution:** disk persistence makes stale-cache debugging harder. Add `--no-doccache` flag and `excel.discover { force: true }` invalidation. Don't persist if the file path is empty or appears to be a temp file.

## Phase C — Auto-diagnostics enrichment (Feature 6)

Goal: one round-trip recovery instead of three.

- Add `classifyOfficeJSErr(err *officejs.OfficeError, env *tools.RunEnv) *tools.EnvelopeError` next to `classifyAcquireErr` in `internal/tools/dispatcher.go:162`.
- Switch on `OfficeError.Code`:
  - `ItemNotFound` (range/sheet not found): inject `Details["available_sheets"]`, `Details["nearest_name_suggestions"]` (Levenshtein on user input), `Details["failing_address"]`. Recovery hint: "Sheet X not found. Did you mean Y? Available: [...]".
  - `InvalidArgument` on a range address: parse the address, inject column/row out-of-bounds detail.
  - Outlook compose-vs-read mismatch: inject `Details["item_mode"]`.
  - PowerPoint slide-index OOB: inject `Details["slide_count"]`.
- Source for "available sheets" lookup: query the doccache (Phase B) first; fall back to a one-shot `excel_list_worksheets.js` call if cache is cold and the budget allows. Cap enrichment to one extra CDP round-trip per error to bound cost.
- Wire by wrapping `tool.Run(...)` in `Dispatcher.Dispatch` (line 145): if the result is an `OfficeError`-shaped failure, call `classifyOfficeJSErr` to enrich `Details`. Per-tool code stays thin.
- Update golden fixtures in `internal/tools/testdata/golden/` to include enriched `Details` for the common error shapes. Add new fixtures for ItemNotFound + Outlook mode + slide OOB.

## Phase D — MCP Resources + change subscriptions (Feature 4)

Goal: the model references workbooks, slides, mail folders by URI; tool calls don't round-trip data through every prompt.

- Define a resource URI grammar (mirrors Phase A target/source grammar): `office://excel/<workbook>/<sheet>!<range>`, `office://pp/<deck>/slide<N>`, `office://outlook/<folder>`, `office://word/<doc>/bookmark/<name>`, `office://onenote/<notebook>/<section>/<page>`.
- New package `internal/resources/` with `Provider` interface (`List`, `Read`, `Subscribe`). One provider per host. Backed by the same JS payloads + doccache.
- Register with the SDK in `internal/mcp/server.go` after tool registration (line 84). Use `s.sdk.AddResource(...)` and `s.sdk.AddResourceTemplate(...)`.
- **Subscriptions** are the hard part. Office.js change events:
  - Excel: `worksheet.onChanged`, `workbook.onSelectionChanged`. Wire via a long-lived JS subscriber that posts to a CDP-injected callback (existing `internal/js/_preamble.js` already runs in the page).
  - Outlook: `Office.context.mailbox.addHandlerAsync(Office.EventType.ItemChanged, ...)`.
  - PowerPoint/OneNote: poll on a timer; no native change events for slides/pages.
- Bridge: a per-session `EventBuf` (already exists — see `RunEnv.EventBuf` in dispatcher.go:305) catches the events; a goroutine drains them and emits `notifications/resources/updated` over the SDK.
- This is the largest piece of new code in the plan. Phase it as: (D1) read-only resources without subscriptions, (D2) Excel change subscriptions, (D3) Outlook + polling fallback for the rest. Ship D1 first; D2/D3 can land later in the same plan or split.

## Phase E — Record/replay macros (Feature 7)

Goal: an organic library of stable workflows the agent can call as one tool instead of re-deriving 20 steps.

- New package `internal/recorder/` with `Store` (file-backed at `%LOCALAPPDATA%\office-addin-mcp\macros\<name>.json`).
- Recording: a new lifecycle tool `macro.record_start { name }` flips `RunEnv.Recording = true`. Dispatcher's pre-call hook (extend dispatcher.go:145) appends `{tool, params}` per call to the active recording. `macro.record_stop` flushes to disk.
- Replay: dynamic registration. On startup, `internal/recorder/Store.Load()` walks the macro dir and registers each as a `tools.Tool` via the existing registry — name `macro.<recorded_name>`, schema = the union of param schemas observed during recording (or just `additionalProperties: true` initially), `Run` = sequentially dispatch the recorded calls.
- Param substitution: simple — recordings capture literal params. v1 doesn't generalize. Document this; users curate macros by re-recording.
- Safety: macros only replay tools currently in the registry. If a tool was renamed, `macro.<name>` returns a clear error pointing at the missing dependency.

## Phase summary / sequencing

| Phase | Feature(s) | Risk | Lands when |
|---|---|---|---|
| 0 | Removal + narrowing | Low | First — frees the surface for the rest |
| A | 1, 3 | Medium (lots of payloads) | After 0 |
| B | 2, 5 | Medium (cache invalidation correctness) | After A; query engine reuses workflow tools |
| C | 6 | Low | Parallel-able with B; depends on doccache for `available_sheets` lookup |
| D | 4 | High (new transport surface, subscriptions) | After A. D1 is small; D2/D3 are bigger and can split. |
| E | 7 | Medium | After C (so recordings carry diagnostics) |

## Critical files to modify

Removal:
- `internal/tools/cdptool/**` — delete
- `cmd/gen-cdp-tools/**` — delete
- `cmd/office-addin-mcp/main.go` — drop `--expose-raw-cdp` + env var
- `internal/mcp/registry.go` — drop `CDPSelection`, simplify `DefaultRegistry`
- `internal/tools/{exceltool,wordtool,outlooktool,powerpointtool,onenotetool}/register.go` — unregister primitives
- `README.md`, `CLAUDE.md`, `CHANGELOG.md`

New code:
- `internal/tools/officetool/` — cross-host workflows (Phase A)
- `internal/js/excel_apply_diff.js`, `excel_query.js`, `excel_summarize_workbook.js`, `word_restructure_outline.js`, `word_apply_edits.js`, `outlook_triage_inbox.js`, `outlook_draft_reply.js`, `powerpoint_rebuild_slide.js`, `onenote_append_to_page.js`, `_query.js` (Phase A/B)
- `internal/doccache/` — persistent context cache (Phase B)
- `internal/tools/dispatcher.go` — add `classifyOfficeJSErr`, wrap `tool.Run` to enrich (Phase C)
- `internal/resources/` — resource providers (Phase D)
- `internal/mcp/server.go` — wire `AddResource` + notifications (Phase D)
- `internal/recorder/` — macro store (Phase E)
- `internal/tools/macrotool/` — `macro.record_start/stop` lifecycle tools (Phase E)
- `internal/tools/result.go` — keep `EnvelopeError.Details` typed-or-map; only add fields if needed for record/replay metadata

## Verification

Per phase:

Phase 0
- `go test ./...` green; `golangci-lint run` green.
- `office-addin-mcp list-tools` output diff: no `cdp.*`, no host primitives. Capture as a fixture so future drift is caught.
- Manual: launch Excel with `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=9222"`, run `excel.runScript` end-to-end against a real workbook to confirm no infrastructure regression.

Phase A
- Per-tool unit tests for schema + summary. Golden envelope fixtures for the success path.
- Manual live-Excel sweep: `excel.tabulate_region` on a real range, `excel.apply_diff` with 5 patches, `office.embed` from Excel → PowerPoint.
- Explicitly state: live Office verification is manual; CI does not exercise it.

Phase B
- `excel.query` correctness: golden tests with a fixed dataset run through the JS query engine in a Node harness so we don't need Excel for unit tests.
- doccache: hash invalidation test (mutate fingerprint → next read misses cache).
- Manual: open a 100k-row workbook, confirm `excel.query` returns ≤100-row result without payload truncation.

Phase C
- New golden fixtures for `ItemNotFound`, `InvalidArgument`, Outlook mode mismatch, slide OOB. Assert `Details` fields populated.
- Bound check: enrichment never emits >1 extra CDP round-trip (instrument `CDPRoundTrips` in fixture).

Phase D
- Resource list/read covered by SDK conformance tests (mcp-go has test harnesses).
- Manual: subscribe an MCP client (Claude Desktop / Inspector) to a workbook resource, edit a cell, observe `notifications/resources/updated`.
- Polling-fallback resources: assert poll cadence configurable + bounded.

Phase E
- Record a 5-step Excel session, replay; assert byte-identical envelopes (modulo timestamps + requestId).
- Negative test: record macro, delete a referenced tool from registry, replay → clear error.

## Out of scope / explicit non-goals

- Reintroducing raw CDP behind a different flag.
- Generalized macro parameterization (template vars in recordings) — v2.
- True ETag-based change detection — Office.js doesn't expose ETags; we use a fingerprint approximation.
- Web-app version of the server. Scope stays Windows + WebView2.
