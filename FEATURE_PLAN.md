# office-addin-mcp — Feature Implementation Plan

## Context

`NEW_FEATURES.md` is the prioritized improvement list for `office-addin-mcp` — a Go MCP server bridging an Office add-in's WebView2 to MCP clients via CDP. Today the project ships v0.2.0 to npm, registered the MCP package, has ~38 test files all passing, and exposes ~60 high-level (`excel.*`, `page.*`, `pages.*`, `addin.*`) tools plus optionally ~411 raw `cdp.*` tools.

Recurring pain points across the nine items: (a) the bridge is hard to bring up because Excel must be hand-launched with a magic env var, (b) the MCP surface still returns text-only payloads when the spec now allows structured content, (c) docs/CLI/version metadata have drifted, (d) per-session locking caps parallel CDP throughput, (e) errors are terse, (f) the raw CDP surface is noisy, (g) the server is silent (no slog, no request IDs, one un-recovered goroutine), and (h) there are no benchmarks or live-Office smoke tests.

The plan groups the nine features into four execution waves chosen for low coupling within waves and clear dependencies between them. Each wave can ship independently behind the existing test suite.

---

## Implementation order

| Wave | Features | Why this order |
| --- | --- | --- |
| **1 — Hygiene** | F4 (contract drift) | Must precede any release that touches public surface; trivially testable. |
| **2 — Observability foundation** | F8 (slog + request IDs + panic recovery), F6 (rich error envelopes) | F6 leans on the request-ID infrastructure F8 introduces; both are pre-reqs for debugging the larger refactors. |
| **3 — DX & semantics** | F1 (auto-launch / detect), F2 (Windows scan + `addin.status`), F3 (StructuredContent / OutputSchema / Annotations), F7 (curate cdp.* surface) | Each is mostly additive; F2's `addin.status` is the natural place to surface F1 detection results. F3 should land before F7 because the codegen template needs the new Tool fields. |
| **4 — Performance** | F5 (concurrent CDP), F9 (benchmarks + live Office smoke) | F9 produces the numbers that prove (or disprove) F5 was worth it; F5 is the riskiest refactor and benefits from the F8 logging being already in place. |

---

## Feature plans

### F4 — Eliminate contract drift (Wave 1)

**Problem.** `mcp.json` says v0.2.0, npm `npm/main/package.json` says v0.1.1, README header says v0.1.0, `cmd/office-addin-mcp/main.go:24` defaults to `0.0.0-dev`. README still advertises `call`, `daemon`, and `serve --stdio` subcommands which `main.go:59-63` now rejects.

**Recommended approach.** Make stdio-only the explicit, documented contract. Single source of truth for version is the git tag; release workflow already injects via `-ldflags -X main.version=...`.

**Files to edit**

- `README.md` — rewrite §"Features" (drop "Daemon mode" and "Stdio mode" duality, keep only stdio), §Install, §Usage examples (lines ~162-188), and the subcommand table (~lines 233-236). Replace v0.1.0 banner with a "see the latest release on GitHub" pointer (no hard-coded version).
- `npm/main/package.json` and the five `npm/<platform>/package.json` files — already auto-rewritten by the release workflow per the implemented release automation; verify the workflow truly mutates all six and add a CI check.
- `mcp.json` — leave the version field but document in the release workflow comment that it's also re-stamped from the tag.
- `.github/workflows/release.yml` — add a pre-publish step that fails if any of {`mcp.json` version, all `npm/*/package.json` versions, `main.go` `-ldflags` value} disagree with the tag, or if README contains `call/daemon/serve --stdio` code fences.

**Reuse.** The release workflow already extracts the version and stamps npm packages; just expand its scope and add the assertion step.

**Verify.**

1. `go test ./...` (no behavior change so all green).
2. `git diff` shows README no longer references removed subcommands.
3. Tag a `v0.2.1-dev.1` against a fork; confirm the new pre-publish lint fails when artificially desyncing one file.

---

### F8 — Structured logging, request correlation, panic recovery (Wave 2)

**Problem.** No `log/slog` anywhere in the tree. `cmd/office-addin-mcp/main.go:67-74` writes plain `fmt.Fprintf` lines to a flat file. `internal/cdp/connection.go:121-159` `readLoop` has no `defer recover()`; ditto `internal/launch/devserver.go:186,190,211` and `internal/session/manager.go` `gcLoop`. `Diagnostics` (`internal/tools/result.go:51-65`) has no `RequestID`.

**Recommended approach.** Adopt `log/slog` with a JSON handler at the top of `main.go`; thread a generated request ID through `tools.Request → tools.Diagnostics → cdp.Connection.Send` via `context.Value`; add `defer recover()` to every long-lived goroutine.

**Files to edit / create**

- `cmd/office-addin-mcp/main.go` — initialize `slog.New(slog.NewJSONHandler(logSink, &slog.HandlerOptions{Level: slog.LevelInfo}))` and call `slog.SetDefault`. Add `--log-level` flag (default `info`).
- `internal/log/log.go` *(new)* — small helper exposing `WithRequestID(ctx, id)` and `RequestID(ctx) string`. Single context key. Keeps `internal/tools` from importing slog directly so test code stays cheap.
- `internal/tools/result.go` — extend `Diagnostics` with `RequestID string \`json:"requestId,omitempty"\``. **Bump `EnvelopeVersion` to `v0.3`.** Update golden fixtures (any `*_golden.json` that asserts the diagnostics shape).
- `internal/tools/dispatcher.go` — at the top of `Dispatch`, generate a request ID with `crypto/rand` (8 bytes hex), stash on context with `internallog.WithRequestID`, copy into `diag.RequestID`. Wrap the call in `slog.Debug("dispatch", "tool", req.Tool, "request_id", id)` with success/error timing.
- `internal/cdp/connection.go` — `readLoop` opens with `defer func(){ if r := recover(); r != nil { c.closeWithErr(fmt.Errorf("readLoop panic: %v", r)) } }()`. Same pattern in `internal/session/manager.go` (`gcLoop`) and the three goroutines in `internal/launch/devserver.go`.
- `internal/cdp/connection.go` `Send` — pull request ID off the context and tag log entries with it.
- *Optional, deferred to a follow-up:* OpenTelemetry wrapper. Mention in the plan but do **not** add the dep in this wave — the slog+request-ID layer is enough to debug today.

**Reuse.** No external deps beyond stdlib. `crypto/rand` + `encoding/hex` is enough for an 8-byte ID; UUID is overkill here.

**Verify.**

1. `go test ./...` — golden fixtures updated; new `TestDispatchStampsRequestID` in `internal/tools/dispatcher_test.go`.
2. `golangci-lint run`.
3. New `TestReadLoopRecoversFromPanic` in `internal/cdp/connection_test.go`: inject a panic via a stub frame and assert the connection closes cleanly with the panic message in the surfaced error.
4. Manual: run with `--log-level=debug`, observe one JSON log line per Dispatch with `tool`, `request_id`, `duration_ms`.

---

### F6 — Actionable error envelopes (Wave 2)

**Problem.** `internal/tools/dispatcher.go:112-127` returns `session_acquire_failed` with just code/message/category/retryable. The AI client cannot self-recover.

**Recommended approach.** Add a typed `RecoveryHint` field plus a small set of well-known detail keys. Construct hints close to the failure site so they have local context.

**Files to edit / create**

- `internal/tools/result.go` — add to `EnvelopeError`:
  ```go
  RecoveryHint string         `json:"recoveryHint,omitempty"`
  Details      map[string]any `json:"details,omitempty"` // already exists; document standard keys
  ```
  Document standard detail keys in the package doc: `probedEndpoint`, `lastKnownTargets`, `manifestUrl`, `expectedUrlPattern`, `recoverableViaTool` (e.g. `"addin.launch"`).
- `internal/tools/dispatcher.go` — when `session.Acquire` returns reconnect-budget-exhausted, populate `Details["probedEndpoint"]` from `req.Endpoint` and `RecoveryHint` like `"Excel may not be running with --remote-debugging-port=9222. Call addin.launch or restart Excel."` Differentiate timeout vs. dial-failure vs. budget-exhausted (the message string is currently the only signal — wrap with sentinel errors in `internal/session`).
- `internal/session/session.go` — export `ErrReconnectBudgetExhausted` and `ErrDialFailed` so `dispatcher.go` can branch with `errors.Is` instead of substring matching.
- `internal/tools/addintool/errors.go` — extend `mapPayloadError` to fill `RecoveryHint` for `RequirementSetUnsupported`, `ManifestMismatch`, `OfficeNotReady`.
- `internal/cdp/connection.go` — when `Send` returns a protocol error, surface the CDP `error.code`/`error.message` already in the frame as `Details["cdpError"]` so the dispatcher can pass it through.

**Reuse.** Existing `Details map[string]any` already exists; this feature is mostly population, not new fields.

**Verify.**

1. New table-driven test `TestEnvelopeErrorRecoveryHints` covering the four primary failure modes.
2. Update relevant golden JSON fixtures; bump `EnvelopeVersion` (already happening for F8).
3. Manual: kill Excel mid-session, hit any tool, observe the envelope's `recoveryHint` says to relaunch.

---

### F1 — Auto-discover / auto-launch Excel (Wave 3)

**Problem.** Users must manually launch Excel with `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=9222"` every session. `internal/webview2/scan_windows.go:7-12` is a stub. CLAUDE.md previously forbade auto-launch — this is the file the user is now overriding via `NEW_FEATURES.md` #1.

**Recommended approach.** Two complementary mechanisms:

1. **Detect-first diagnostic** (no spawning): fill in the Windows scan so the discovery ladder picks up an existing Excel that's already running with the right port even when the user didn't pass `--browser-url`. If we detect Excel is running *without* the debug port, surface a friendly diagnostic (not a silent failure).
2. **Opt-in auto-launch**: a new `--launch-excel` flag plus an `addin.ensureRunning` tool that calls `addin.detect` → `addin.launch` if Excel isn't reachable.

**Files to edit / create**

- `internal/webview2/scan_windows.go` — implement the scan (see F2 below; F1 and F2 share this file).
- `cmd/office-addin-mcp/main.go` — new `--launch-excel` flag. When set and `webview2.Discover` fails on startup, attempt `launch.LaunchExcel(...)` (existing function in `internal/launch/launcher.go:79-164`) before starting the MCP server.
- `internal/launch/launcher.go` — extract a `LaunchIfNeeded(ctx, manifest)` helper that probes the endpoint first, only spawns if needed, and returns the resolved endpoint. Reuses existing `waitForCDPReady`.
- `internal/tools/addintool/ensurerunning.go` *(new)* — `addin.ensureRunning` tool: (a) probe current endpoint via `webview2.Discover`, (b) if reachable return `{state: "ready", target}`, (c) if unreachable and a manifest is known via `RunEnv.Manifest()`, call into the same launch path as `addin.launch`, (d) if no manifest, return error with `RecoveryHint` pointing at `addin.detect`.
- `internal/mcp/registry.go` — register the new tool.
- `CLAUDE.md` — replace the "Auto-launch is not implemented" note with the actual behavior: "auto-launch is opt-in via `--launch-excel` or the `addin.ensureRunning` tool".

**Reuse.** `internal/launch/launcher.go` already does the heavy lifting (manifest detection, env var, `office-addin-debugging` invocation, CDP readiness polling). This feature is wiring + a friendly entry point.

**Verify.**

1. `go test ./...`.
2. New unit test for `LaunchIfNeeded` using a stub launcher.
3. Manual smoke: with Excel closed, run `office-addin-mcp --launch-excel`, send `addin.detect` then any `excel.*` tool — should work without separately calling `addin.launch`.

---

### F2 — Resilient WebView2 discovery + `addin.status` (Wave 3)

**Problem.** `scan_windows.go` is a stub. There's no first-class status tool: the AI has to chain `addin.detect` + `addin.listTargets` + `addin.contextInfo` to figure out what's wrong.

**Recommended approach.** Implement Windows scan via three layers in priority order: known WebView2 user-data dirs → process command-line scrape → `tasklist /v` fallback. Add `addin.status` that aggregates everything an AI needs to decide whether to retry, relaunch, or give up.

**Files to edit / create**

- `internal/webview2/scan_windows.go` — replace the stub:
  1. **User-data-dir scan** — enumerate likely paths: `%LOCALAPPDATA%\Microsoft\Office\<app>\WebView2\<package>\EBWebView\Default\DevToolsActivePort`. The Office Add-in user-data dirs follow a pattern under `%LOCALAPPDATA%\Microsoft\Office`. Read each `DevToolsActivePort` file (line 1 = port, line 2 = browser WS path), assemble the WS URL, probe `/json/version` to confirm liveness.
  2. **Process scan** — `wmic process where "name='Excel.exe'" get CommandLine /value` (PowerShell-friendly; already used elsewhere in the repo per the launcher). Look for `--remote-debugging-port=NNNN` in the command line. (Or use `WMIC` via `internal/launch/procattr_windows.go` patterns.)
  3. Return the first endpoint that responds to `/json/version`, ranked by liveness.
- `internal/webview2/scan_windows_test.go` *(new)* — table-driven tests against fixture `DevToolsActivePort` files in `testdata/`.
- `internal/tools/addintool/status.go` *(new)* — `addin.status` tool returning:
  ```json
  {
    "endpoint": {"source": "scan|browser-url|ws-endpoint|default", "wsURL": "...", "browserURL": "..."},
    "targets": [{"id": "...", "url": "...", "type": "page", "matchesManifest": true}],
    "selectedTarget": {...},
    "manifest": {"loaded": true, "id": "...", "url": "..."},
    "office": {"ready": true, "host": "Excel", "platform": "PC", "version": "16.0...", "requirementSets": ["ExcelApi 1.16", ...]},
    "recoveryHints": []
  }
  ```
  Run order: discover → list targets → for each Office target, evaluate `Office.context` to grab host/platform/requirement sets. Return partial results on failure with explanatory `recoveryHints[]`.
- `internal/mcp/registry.go` — register `addin.status`.

**Reuse.** `internal/cdp/discovery.go ResolveBrowserWSURL`, `internal/webview2/discover.go` priority ladder, `internal/tools/addintool/contextInfo` for the Office probe — most of the building blocks exist; this is composition.

**Verify.**

1. Unit tests on the scanner with synthetic `DevToolsActivePort` files.
2. Integration: the existing `internal/cdp/integration_test.go` already creates a real DevToolsActivePort; reuse its setup to drive a test against `addin.status`.
3. Manual: with Excel running but no add-in loaded, `addin.status` should report `manifest.loaded=false` and a recovery hint suggesting `addin.detect` or `addin.launch`.

---

### F3 — MCP-native structured results (Wave 3)

**Problem.** `internal/mcp/adapter.go:21-27` registers only Name/Description/InputSchema. `adapter.go:56-67` only emits TextContent (or ImageContent for screenshots). Spec at https://modelcontextprotocol.io/specification/2025-11-25/server/tools supports `structuredContent`, `outputSchema`, `title`, `annotations`, `icons`.

**Recommended approach.** Extend `tools.Tool` with new optional fields, populate them in the high-level tools, and have the adapter emit `StructuredContent` whenever `OutputSchema` is set, while keeping TextContent as a compatibility fallback.

**Files to edit / create**

- `internal/tools/registry.go` — extend the `Tool` struct:
  ```go
  Title        string          // human-readable
  OutputSchema json.RawMessage // optional; when set, results emit StructuredContent
  Annotations  *Annotations    // {readOnlyHint, destructiveHint, idempotentHint, openWorldHint}
  Icons        []Icon          // {src, mimeType, sizes}
  ```
  Compile `OutputSchema` like `Schema` does (and validate `env.Data` against it in dev/test builds with a build tag — production skips for performance).
- `internal/mcp/adapter.go` — `registerTool` copies the new fields onto `sdk.Tool`. `envelopeToResult` emits `res.StructuredContent = env.Data` when `OutputSchema` is non-empty; keeps the JSON `TextContent` as a fallback (per spec, both can coexist).
- `internal/tools/exceltool/*.go`, `pagetool/*.go`, `addintool/*.go` — populate `Title` and `Annotations` (`readOnlyHint=true` for `excel.getRange`, `addin.status`, `page.snapshot`; `destructiveHint=true` for `excel.run`, `Browser.crash`, etc.). Define `OutputSchema` for the high-value tools first (top 10 by usage): `excel.getRange`, `excel.listTables`, `page.snapshot`, `addin.status`, `addin.listTargets`, `excel.runScript`, `pages.list`, `pages.select`, `page.screenshot` (already image content), `addin.contextInfo`. Lower-priority tools can land later.
- `cmd/gen-cdp-tools/template.go` — for generated `cdp.*` tools, populate `Annotations` from the protocol JSON (`category` field maps to read-only/destructive). Output schemas are mostly already present in the protocol JSON's `returns` blocks — emit those as `OutputSchema`.
- Update goldens for affected tools.

**Reuse.** Existing `imageFromData` shows the adapter already specializes content blocks; the new `StructuredContent` code path slots in next to it. The codegen pipeline's manifest layer (`cmd/gen-cdp-tools/manifest.yaml`) is the right place for declarative annotation overrides.

**Verify.**

1. `go test ./internal/tools/... ./internal/mcp/...`.
2. New `TestEnvelopeToResultEmitsStructuredContent` in `internal/mcp/adapter_test.go`.
3. Manual MCP client smoke (Cursor or `mcp inspect`): list tools, confirm `outputSchema`, `annotations`, `title` fields appear; call `addin.status`, confirm the structured object is delivered.

---

### F7 — Curate the 411 cdp.* tool surface (Wave 3)

**Problem.** Even with `--expose-raw-cdp=false`, when users do enable it they get all 411 at once, costing thousands of tokens in tool listings. The high-level `excel.*` / `page.*` / `addin.*` descriptions are terse.

**Recommended approach.** Three-pronged:

1. Keep `--expose-raw-cdp=off` default (already done).
2. Add `--cdp-domains=DOM,Page,Runtime,Input` flag that, when set, *implies* exposure but only registers the named domains.
3. Enrich high-level tool descriptions with concrete examples in their schemas (using JSON Schema's `examples` array).

**Files to edit / create**

- `cmd/office-addin-mcp/main.go` — new `--cdp-domains` flag (string, comma-separated). When non-empty, treat `--expose-raw-cdp` as implicitly true. New `--list-cdp-domains` flag prints available domains and exits.
- `internal/mcp/registry.go` `DefaultRegistry` — change signature to accept a `CDPSelection` struct: `{Enabled bool, Domains []string}`. Pass to `cdptool.Register`.
- `internal/tools/cdptool/register.go` — accept the domain filter; iterate `RegisterGenerated`'s domain registers and skip those not in the allow-list. The 18 domain register functions in `internal/tools/cdptool/generated/register_generated.go` are already separated, so filtering is a switch-statement.
- `internal/tools/cdptool/generated/register_generated.go` — codegen output. Update `cmd/gen-cdp-tools/template.go` to emit a per-domain register and a domain catalog (`var Domains = []string{"DOM", "Page", ...}`).
- `internal/tools/exceltool/*.go`, `pagetool/*.go`, `addintool/*.go` — enrich the most-used tools' Description with a one-line example block ("Example: `{address: 'Sheet1!A1:B5'}` returns the values as a 2-D array."). Add `examples` field to their input schemas.
- `cmd/gen-cdp-tools/manifest.yaml` — add a `recommended_domains` list documenting which domains are usually safe for Office automation (DOM, Page, Runtime, Input, Network, Target). Emit a comment block in the generated register file referencing it.

**Reuse.** The codegen at `cmd/gen-cdp-tools/` already has domain awareness via the protocol JSON; this is reshaping its output.

**Verify.**

1. `go generate ./...` regenerates the cdp.* register; `go test ./cmd/gen-cdp-tools/...` golden tests pass.
2. `go test ./...`.
3. Manual: run with `--cdp-domains=DOM,Page,Runtime`, list tools, confirm only those domains' cdp.* tools appear plus all high-level tools.

---

### F5 — Concurrent CDP calls per session (Wave 4)

**Problem.** `internal/session/session.go:66` uses `sync.Mutex` and `Acquire` (line 137) holds it for the *entire* tool call. AI batched parallel calls (e.g., read range + list tables + screenshot in parallel) serialize against each other on the same session.

**Recommended approach.** This is structural, so do it carefully. The mutex protects multiple things: connection pointer, selection cache, snapshot, `enabled` map, `eventBufs`, reconnect budget. Splitting by concern:

- **Connection lifecycle (dial/reconnect):** `sync.RWMutex`. Write-locked during dial/reconnect/close. Read-locked during steady-state command dispatch.
- **Per-resource state (selection, snapshot, enabled, eventBufs):** keep their own narrower locks (`sync.Mutex` per map). These are touched by tool handlers between Acquire and release; they're cheap enough that fine-grained locks won't measurably slow them.
- **Reconnect budget:** stays under the connection write lock — only consulted on reconnect.

**Files to edit**

- `internal/session/session.go`:
  - Replace `mu sync.Mutex` with `connMu sync.RWMutex` (connection lifecycle), `stateMu sync.Mutex` (selection/default/snapshot/enabled), `eventMu sync.Mutex` (eventBufs).
  - `Acquire` returns the conn after taking a *read* lock on `connMu`. Release drops the read lock. The dial path upgrades to a write lock (drop read, take write, double-check, dial, downgrade).
  - `Selected`, `SetSelected`, `InvalidateSelection`, `SetDefaultSelection`, etc. become self-locking on `stateMu` (drop the "must be called with the session lock held" comments).
  - `EnsureEnabled` synchronizes per (cdpSessionID, domain) under `stateMu` — but the underlying `Send` call to `<Domain>.enable` happens *outside* the lock; use a `sync.Once`-per-pair pattern via `singleflight.Group` (`golang.org/x/sync/singleflight` — verify in `go.mod`, it's a tiny dep).
  - `dropConnLocked` is invoked under the connection write lock; clears state under `stateMu`.
- `internal/tools/dispatcher.go` `buildRunEnv` — no caller-visible change beyond removing comments asserting "lock held".
- All callers of `sess.Selected`/`SetSelected`/`Snapshot`/`SetSnapshot`/etc. — drop "the lock is held" assumptions; verify no double-fetch race patterns.
- `internal/session/session_test.go` *(extend)* — new `TestConcurrentAcquireDoesNotSerialize`: spawn N goroutines that each Acquire and run a stub Send; assert wall-clock < N × (single-call time × 0.6).

**Reuse.** `internal/cdp/connection.go` is already concurrent-safe (its `Send` is keyed on per-request id channels); this refactor unlocks the latent capability.

**Verify.**

1. `go test ./internal/session/... -race` — race detector mandatory.
2. `go test ./...` full suite passes.
3. New benchmark `BenchmarkSessionParallelDispatch` (lives with F9) shows ≥ 2× speedup with 4 parallel callers vs. serial.
4. Manual: run an MCP client that issues 4 simultaneous tool calls, observe overlapping `request_id` log entries with overlapping `duration_ms` ranges.

**Risks.** This refactor is the riskiest item. Consider gating behind a `--concurrent-cdp` flag for one release before making default. Keep the old code path in a `_legacy.go` file behind a build tag for a rollback escape hatch.

---

### F9 — Live Office validation + benchmarks (Wave 4)

**Problem.** `internal/cdp/integration_test.go:78-158` is the only integration test, uses headless Chrome (not Excel), and is skipped under `-short` because it's flaky on Windows CI. No benchmarks anywhere.

**Recommended approach.** Make integration tests opt-in via tag, add an Office.js smoke test that runs against a sample workbook locally (manual but reproducible), and add benchmarks for the four perf-critical paths.

**Files to edit / create**

- `internal/cdp/integration_test.go` — keep behind `//go:build integration` so it never runs on CI but is trivially `go test -tags integration ./internal/cdp/...` for devs. Tighten the DevToolsActivePort wait by polling the file *and* probing `/json/version` simultaneously, with a 30s ceiling.
- `internal/officejs/integration_test.go` *(new, build-tagged)* — drives a sample workbook + add-in (paths configured via `OAMCP_TEST_MANIFEST` and `OAMCP_TEST_WORKBOOK` env vars). Smoke covers: Office readiness, `excel.getRange`, `excel.runScript`, `excel.listTables`. Documents in package doc how to set up the env vars.
- `testdata/sample-addin/` *(new)* — minimal Office add-in manifest.xml + a tiny static taskpane HTML. Used by the integration test and the benchmarks.
- `internal/tools/dispatcher_bench_test.go` *(new)*:
  - `BenchmarkDispatchNoSession` (lifecycle tools — measures pure dispatch overhead).
  - `BenchmarkDispatchSessionWarm` (a noop tool with a real session — warm path, the selector-cache hit case).
  - `BenchmarkSessionParallelDispatch` (validates F5).
- `internal/officejs/executor_bench_test.go` *(new)* — measures Office.js payload latency: `BenchmarkOfficeJSGetRange` against the sample workbook.
- `internal/cdp/connection_bench_test.go` *(new)* — `BenchmarkCDPSendRoundTrip` against headless Chrome.
- `internal/officejs/executor.go` — add internal latency stamps so the existing `Diagnostics` could carry `payloadLatencyMs` (separate from total `durationMs`). Optional: gate behind a `--diag-verbose` flag to keep envelopes lean by default.
- `.github/workflows/ci.yml` — new job `bench` (manual `workflow_dispatch` only) that runs `go test -bench=. -benchmem -run=^$ ./...`, captures baseline numbers as an artifact for trending.

**Reuse.** `internal/cdp/integration_test.go`'s setup helpers (`launchChrome`, `readDevToolsPort`) are reusable for the new Office test in spirit — though Office launch will use `internal/launch/launcher.go LaunchIfNeeded` (introduced in F1).

**Verify.**

1. `go test ./...` (default, `-short`-friendly) — fast suite still passes.
2. `go test -tags integration ./internal/cdp/...` and `./internal/officejs/...` — integration green when env vars set.
3. `go test -bench=. ./internal/tools/... ./internal/officejs/... ./internal/cdp/...` — produces a baseline; commit baseline `bench.txt` to track regressions.

---

## Cross-cutting verification matrix

For every wave:

1. `go test ./...` — fast suite green (with `-race` on the F5 wave).
2. `golangci-lint run` — clean.
3. Affected golden JSON fixtures regenerated and committed.
4. `EnvelopeVersion` bump if the envelope shape changed (F6, F8 → v0.3; F3 → v0.4 if `StructuredContent` lands).
5. README, CLAUDE.md, and CHANGELOG updated for any user-visible change.
6. Manual smoke against a real Excel + add-in for waves 3 and 4 (the live-Office wave can be validated with the F9 sample workbook).

## Critical files reference

| Concern | File | Notes |
| --- | --- | --- |
| Tool struct | `internal/tools/registry.go:15-28` | F3, F7 add fields |
| Adapter | `internal/mcp/adapter.go:21,56` | F3 emits StructuredContent |
| Envelope | `internal/tools/result.go:39-65` | F6 RecoveryHint; F8 RequestID |
| Dispatcher | `internal/tools/dispatcher.go:66-156` | F6 enrich errors; F8 stamp request id |
| Session | `internal/session/session.go:60-186` | F5 lock split |
| CDP read loop | `internal/cdp/connection.go:121-159` | F8 panic recovery |
| Windows scan stub | `internal/webview2/scan_windows.go:7-12` | F1, F2 implement |
| Launch helper | `internal/launch/launcher.go:79-164` | F1 reuse via LaunchIfNeeded |
| Main | `cmd/office-addin-mcp/main.go:23-130` | F1, F4, F7, F8 flags |
| Codegen | `cmd/gen-cdp-tools/template.go` | F3, F7 |
| MCP registry | `internal/mcp/registry.go` | F1, F2, F7 register new tools / accept domain selection |
| Release workflow | `.github/workflows/release.yml` | F4 drift check |
| Versions | `mcp.json:9`, `npm/*/package.json` | F4 |
| README | `README.md:7,15,162-188,233-236` | F4 rewrite |
