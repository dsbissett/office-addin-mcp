# office-addin-mcp — Go Implementation Plan

## Context

`C:\repos\excel-webview2-mcp` is a TypeScript MCP server (forked from chrome-devtools-mcp) that drives Excel Office add-ins running inside WebView2 over Chrome DevTools Protocol. It works, but it carries fork weight: a ~200MB `chrome-devtools-frontend` dependency, puppeteer's full browser-lifecycle abstraction, 70+ tools many of which are general DevTools features rather than Office-add-in automation, and a single stdio-only process model that re-pays connection cost on every invocation.

We want a leaner Go substrate at `C:\repos\office-addin-mcp` that owns the transport (CDP/WebSocket, target discovery, runtime contexts, sessions) and treats Office.js as **payloads injected through CDP**, not as a JavaScript runtime to integrate with. The win is: faster cold start, persistent sessions across CLI calls (daemon mode), a small stable tool surface designed for AI agents (`{ok, data, error, diagnostics}`), and clean seams to add MCP protocol and more Office hosts later. This is a redesign informed by the TS repo, not a port.

**Confirmed decisions (from clarifying questions):**
- CDP client: hand-rolled minimal client over `gorilla/websocket` (no chromedp/go-rod).
- MCP protocol: deferred. v1 ships CLI + daemon with our own JSON envelope.
- Daemon IPC: local TCP on `127.0.0.1` with a token-auth session file.
- JS payloads: authored as real `.js` files under `/internal/js`, embedded with `//go:embed`.

---

## 1. Current-state analysis (TS repo)

**What it does:** stdio MCP server. On each tool call: ensure browser connected (puppeteer over `http://127.0.0.1:9222` by default, or auto-launch Excel via `office-addin-debugging`), pick a `Page`, evaluate Office.js code, return a `McpResponse`.

**Concepts worth preserving (conceptually, not as code):**
- Single funnel for tool errors with categories (`auth`, `not_found`, `timeout`, `validation`, `connection`, `protocol`, `unsupported`, ...) — `src/tools/ToolError.ts`.
- Reconnect budget (3 reconnects per 60s window) + exponential backoff (`src/connection/retry.ts`, `src/connection/session.ts`).
- Mutex serialization of tool calls (`src/Mutex.ts`) — Office.js `context.sync()` flows do not parallelize cleanly.
- Lifecycle tracking of launched Excel processes keyed by manifest path (`src/tools/lifecycleState.ts`).
- WebView2-specific target filter that does not strip `about:blank` (`src/browser.ts`).
- The pattern of probing `globalThis.Office` + `Excel`, calling `Office.onReady()` with a 1s timeout, and checking `isSetSupported(name, version)` before issuing API calls.

**To redesign, not port:**
- Tool surface. The TS repo has 52 tools, many inherited from chrome-devtools-mcp (lighthouse, performance traces, screencast, emulation, in-page DevTools interop). v1 Go ships ~20 tools focused on Office add-in automation; the rest are explicitly out of scope.
- Result shape. Replace `McpResponse` (markdown text + optional structured content) with a single uniform `{ok, data, error, diagnostics}` JSON envelope.
- CLI flag set. The TS binary has 25+ flags; v1 Go has under 10.

**To discard:**
- `chrome-devtools-frontend` (~200MB, used for trace formatting/Lighthouse).
- Puppeteer entirely; we go straight to CDP.
- DevTools page-interop ("in-page tools"), screencast, lighthouse, memory snapshots, trace insights, emulation.
- rollup bundling, ESLint, Prettier, MCP CLI generation.

**Reused intel:** the inline JS bodies inside `src/tools/excel.ts` are the most valuable artifact. They encode the right requirement-set probes and `Excel.run` patterns. Each `excel.*` Go tool will re-author the equivalent as a standalone `.js` file under `/internal/js`, keeping the same semantics.

---

## 2. Target architecture

**Boundary:** Go owns process lifetime, CDP connection, target/context selection, sessions, tool dispatch, JSON I/O. JavaScript owns Office.js semantics — every `excel.*` payload is a self-contained async function that takes one JSON-serializable arg and returns one JSON-serializable result.

**Layered packages:**

```
cmd/office-addin-mcp/main.go              -- thin: parse flags, dispatch to subcommand
internal/cli/                              -- subcommand implementations (one-shot, serve, call, daemon)
internal/config/                           -- flag + env parsing into Config struct
internal/logging/                          -- leveled logger; stderr only (stdout reserved for results)
internal/cdp/                              -- WebSocket CDP client; one Connection per browser
  connection.go                              -- ws dial, message pump, request/response correlation
  client.go                                  -- typed methods: Target.*, Runtime.*, Page.*, DOM.*
  target.go                                  -- target list, attach, sessionId routing
  runtime.go                                 -- evaluate, callFunction, awaitPromise, exception unwrap
  page.go, dom.go                            -- thin wrappers
internal/webview2/                         -- endpoint discovery + (later) attach
  discovery_windows.go                       -- probe :9222, scan known WebView2 user-data dirs
  discovery_other.go                         -- stub returning ErrUnsupported
internal/session/                          -- stateful: connection + selected target + cached contextIds
  manager.go                                 -- map[sessionID]*Session, GC on idle
  session.go                                 -- one logical session = one CDP connection + selection state
  cache.go                                   -- per-target contextId cache, invalidated on Runtime.executionContextDestroyed
internal/tools/                            -- registry, schema, dispatcher, result envelope
  registry.go schema.go dispatcher.go result.go
internal/tools/cdp/                        -- evaluate, callFunction, getTargets, selectTarget
internal/tools/browser/                    -- navigate, screenshot
internal/tools/dom/                        -- querySelector, click, typeText
internal/tools/excel/                      -- readRange, writeRange, worksheet, table, selection, runScript
internal/officejs/                         -- payload loader + executor (wraps cdp.runtime + injected JS)
  executor.go                                -- Run(ctx, session, payloadName, args) → (json.RawMessage, error)
  payloads.go                                -- //go:embed FS, name→source lookup
  serialization.go                           -- argument marshaling, result+error unwrapping
internal/js/                               -- *.js source files, embedded via //go:embed
testdata/                                  -- sample params + golden response JSON
docs/                                      -- architecture.md, tool-contracts.md, migration-notes.md
```

**One-shot vs daemon — the same code path:**
The CLI subcommand `call` always goes through the same `tools.Dispatcher`. In one-shot mode, `main.go` constructs an in-process `session.Manager` with one ephemeral session, invokes the dispatcher, prints the JSON envelope, exits. In daemon mode, `serve`/`daemon` runs the same `session.Manager` behind a TCP listener (`127.0.0.1:<port>`); the `call` subcommand becomes a thin TCP client that ships a JSON request and writes the response to stdout. **No tool implementation knows which mode it's in.**

**Session model:** a `Session` owns one CDP `Connection`, one currently-selected target, and a per-target context-ID cache. Sessions are addressed by a string ID (`default` if unspecified). Idle sessions are GC'd after a configurable timeout (default 30 min). Sessions reconnect transparently on `disconnected`, with the TS repo's 3-per-60s budget.

---

## 3. Tool contract design

**Naming:** lowercase dotted, `<domain>.<verb><Noun>`. `excel.readRange`, `dom.querySelector`, `cdp.evaluate`. Verbs are camelCase. No underscores (matches the prompt; diverges from TS repo's snake_case).

**Input:** every tool takes a single JSON object. Validation at the boundary using JSON Schema declared next to the tool (`schema.go` per package) — schemas are also surfaced via a `tools.list` meta tool so agents can introspect.

**Output envelope (always, success or failure):**

```json
{
  "ok": true,
  "data": { ... },
  "diagnostics": {
    "tool": "excel.readRange",
    "sessionId": "default",
    "targetId": "...",
    "contextId": 7,
    "durationMs": 37,
    "cdpRoundTrips": 2
  }
}
```

On failure: `ok: false`, `data` omitted, `error: { code, message, category, retryable, details? }`.

**Error categories** (carried forward from TS): `validation`, `not_found`, `timeout`, `connection`, `protocol`, `unsupported`, `office_js`, `internal`. `office_js` is new — Office.js threw inside `Excel.run` (e.g. `InvalidArgument`, `ItemNotFound`).

**Validation:** central `schema.Validate(toolName, raw json.RawMessage) error`. Tools receive a typed, already-validated struct. Validation failures return `ok:false, error.category=validation` without ever touching CDP.

---

## 4. CDP layer design

- **Connection:** `cdp.Dial(ctx, wsURL)` opens a single WebSocket. One reader goroutine demultiplexes by `id` → response chan, by `method` → event subscription. One writer goroutine serializes sends.
- **Endpoint discovery:** `webview2.Discover(ctx, cfg)` returns the WS URL. Order: explicit `--ws-endpoint` > explicit `--browser-url` (probe `/json/version`) > default `http://127.0.0.1:9222` > (Windows only) future scan of WebView2 user-data dirs / process enumeration.
- **Target selection:** `Target.getTargets`. Default heuristic for Excel WebView2: prefer targets whose `url` matches the configured add-in URL pattern, else first target with `type=page` that is not `chrome://` / `edge://` / `devtools://`. Explicitly selectable via `cdp.selectTarget`.
- **Sessions/sessionId:** use `Target.attachToTarget {flatten:true}` and route subsequent commands via the returned `sessionId` (the CDP "flatten" model — single WS, multiple logical sessions).
- **Evaluate + promise awaiting:** `Runtime.evaluate {awaitPromise:true, returnByValue:true, userGesture:true}`. Inspect `result.exceptionDetails` first; surface as `office_js` if the exception text matches Office.js error codes, else `protocol`.
- **Execution contexts:** subscribe to `Runtime.executionContextCreated/Destroyed`; the session cache maps `(targetId, frameId)` → contextId. On stale context, retry once after re-resolving.
- **Frames:** v1 always evaluates in the main frame. Multi-frame is a stretch goal — design the cache key to allow it but do not expose a frame selector yet.
- **Reconnect:** on `disconnected` event from the WS, mark session stale; next tool call attempts reconnect with backoff (500/1000/2000/4000/5000 ms) capped at 3 per 60s. Beyond cap → sticky `connection` error until manual reset.
- **Timeouts:** every tool call carries a context with a default 30s deadline (configurable per call via `--timeout`).

---

## 5. Office.js layer design

- **Storage:** each operation has its own `.js` file in `/internal/js`, e.g. `excel_read_range.js`. The file exports nothing — it is an `async function(args){ ... }` body that `executor.Run` wraps as `(async (args) => { <body> })(<args>)` and hands to `Runtime.evaluate`.
- **Embedding:** `internal/officejs/payloads.go` uses `//go:embed ../js/*.js` and exposes `Get(name string) (string, error)`. Names are the Go tool's name (`excel.readRange` → `excel_read_range.js`).
- **Common preamble:** a single `_preamble.js` is concatenated before every Excel payload. It (a) waits on `Office.onReady()` with a 1s timeout, (b) verifies `globalThis.Excel` exists, (c) declares `requireSet(name, version)` that throws a structured `{__officeError:true, code, message}` if unsupported.
- **Result envelope from JS:** every payload returns `{result: ...}` on success or `{__officeError: true, code, message, debugInfo}` on caught failure. The Go executor unwraps: success → tool's `data`; error → tool's `error` with `category=office_js`.
- **`context.sync` discipline:** payloads `await context.sync()` exactly where needed and return only JSON-serializable values (no `RangeAreas` handles). Tested via node-side dry runs (see Testing).
- **Versioning:** payload files carry a `// @requires ExcelApi 1.4` header comment; `payloads.go` parses these on init and the executor checks at runtime via `requireSet`. This makes capability requirements grep-able and testable.

---

## 6. CLI design

```
office-addin-mcp <subcommand> [flags]

call         --tool <name> --param '<json>' [--session <id>] [--timeout 30s]
                            [--ws-endpoint <url> | --browser-url <url>]
                            (one-shot if no daemon; otherwise routes to daemon)
serve        --stdio        (read JSON requests on stdin, write envelopes on stdout — daemon-equivalent for MCP-style hosts)
daemon       --port 45931 [--token-file <path>] [--idle-timeout 30m]
list-tools                  (prints tool names + JSON Schemas)
status       [--session <id>] (connection state, selected target, contextId)
version
```

- **Flag/env precedence:** flag > env (`OFFICE_ADDIN_MCP_*`) > config file (`%APPDATA%\office-addin-mcp\config.toml` on Windows) > built-in default.
- **Daemon discovery from `call`:** if a daemon socket file exists at the well-known location (`%LOCALAPPDATA%\office-addin-mcp\daemon.json` containing `{port, token}`), `call` connects there; otherwise `call` runs in-process.
- **Auth:** daemon writes a random 32-byte token to the socket file (mode 0600 / Windows ACL'd to current user). Every TCP request must carry `Authorization: Bearer <token>`.
- **Output:** the JSON envelope to stdout; logs to stderr; exit code `0` on `ok:true`, `1` on `ok:false`, `2` on usage errors.

---

## 7. Implementation phases

### Phase 0 — Repo init
- `go mod init github.com/<owner>/office-addin-mcp`, Go 1.22+.
- Skeleton tree per §2; CI on Windows runner; `golangci-lint` + `gofumpt`.
- **Deliverable:** `go build ./...` produces an empty binary that prints `version`.
- **Acceptance:** lint clean; CI green on Windows + Linux.

### Phase 1 — Minimal CDP + Runtime.evaluate
- `internal/cdp` Connection, request/response correlation, event pump.
- `cdp.evaluate` tool with `{expression, awaitPromise, returnByValue}`.
- One-shot CLI only.
- **Deliverable:** `office-addin-mcp call --tool cdp.evaluate --param '{"expression":"1+1"}'` against a `chrome --remote-debugging-port=9222` returns `{"ok":true,"data":{"value":2},...}`.
- **Acceptance:** integration test against headless Chrome in CI passes; unit tests for message correlation, exception unwrap, timeout.

### Phase 2 — Target discovery + WebView2 attach
- `Target.getTargets`, `attachToTarget {flatten:true}`, sessionId routing.
- Tools: `cdp.getTargets`, `cdp.selectTarget`, `browser.navigate`.
- `webview2.discovery_windows.go` (initially: just probe `:9222`).
- **Deliverable:** can attach to Excel WebView2 (manually launched with `--remote-debugging-port`) and evaluate.
- **Acceptance:** manual test against Excel + a trivial add-in returns `globalThis.Office?.context?.host` correctly.

### Phase 3 — Tool registry + result envelope
- `tools.Registry`, `Dispatcher`, JSON Schema validation, uniform envelope, error categories, diagnostics population.
- `list-tools` subcommand.
- **Deliverable:** all Phase 1–2 tools moved onto the registry; envelope identical regardless of tool.
- **Acceptance:** golden-JSON tests for envelope shape pass for success, validation error, CDP error, timeout.

### Phase 4 — First Excel Office.js tools
- `internal/officejs` executor + `_preamble.js` + per-tool payload files.
- Tools: `excel.readRange`, `excel.writeRange`, `excel.listWorksheets`, `excel.getActiveWorksheet`, `excel.activateWorksheet`, `excel.createWorksheet`, `excel.deleteWorksheet`, `excel.getSelectedRange`, `excel.setSelectedRange`, `excel.runScript`, `excel.createTable`.
- **Deliverable:** end-to-end demo against real Excel WebView2 reading and writing a range.
- **Acceptance:** manual checklist in `docs/migration-notes.md` showing parity with corresponding TS tools; golden response JSON for each tool against a fixture workbook.

### Phase 5 — Sessions, daemon, stdio
- `session.Manager`, idle GC, reconnect budget, context cache invalidation.
- `daemon` (TCP+token), `serve --stdio`, `call` autoroutes to daemon when socket file present.
- **Deliverable:** ten sequential `call` invocations against a running daemon reuse one CDP connection (verified via `cdpRoundTrips` diagnostics not paying attach cost after the first).
- **Acceptance:** kill-and-restart-Excel test: daemon survives, next call transparently reconnects within the 3/60s budget.

### Phase 6 — Tests, docs, packaging, migration guide
- `docs/architecture.md`, `docs/tool-contracts.md`, `docs/migration-notes.md` (mapping each TS tool → Go tool / "out of scope" / "deferred").
- Windows MSI or zip release; `goreleaser` config.
- **Deliverable:** v0.1.0 tag.
- **Acceptance:** README quick-start works on a clean Windows VM with Excel + a sample add-in.

---

## 8. Testing strategy

- **Unit:** `cdp` message correlation, exception unwrap, reconnect budget; `tools` schema validation; `officejs` argument marshaling and result/error unwrapping; `session` cache invalidation on `executionContextDestroyed`.
- **Integration (CI):** spin up headless Chrome with `--remote-debugging-port`; exercise `cdp.*`, `browser.*`, `dom.*` tools end-to-end. No Office.js needed at this layer.
- **Office.js payload tests (node):** each `internal/js/*.js` file has a sibling `*.test.mjs` that loads it under a fake `globalThis.Excel`/`Office` (a minimal stub) and asserts the produced result envelope shape. Catches syntax errors and obvious logic regressions without needing Excel.
- **Manual Excel matrix:** documented checklist in `docs/migration-notes.md` for each `excel.*` tool against the same sample workbook used by the TS repo's e2e tests.
- **Golden JSON:** `testdata/golden/<tool>.json` for the envelope shape of each tool — success and at least one error case.
- **Error scenarios:** target gone, context destroyed mid-eval, Excel closed during call, malformed param, Office.js requirement set unsupported, payload syntax error.

---

## 9. Migration strategy

- **Parity table** in `docs/migration-notes.md`: every TS tool → one of `{ported, renamed, deferred, dropped}` with rationale. This is the contract for what "v1 done" means.
- **Behavioral comparison harness:** a small node script that runs the TS server and the Go binary side-by-side against the same Excel session and diffs response payloads for the overlapping tools. Lives under `scripts/parity/`.
- **Drop list (explicit):** lighthouse, performance traces, screencast, in-page DevTools interop, memory snapshots, emulation, network request inspection (deferred to a later phase, not v1).
- **Renames:** snake_case → camelCase dotted (e.g. `excel_read_range` → `excel.readRange`). Documented mapping table.
- **TS-isms to retire:** zod schemas (replaced by JSON Schema files), McpResponse markdown text (replaced by structured `data`), per-tool ad-hoc result shaping (replaced by envelope).

---

## 10. Risk register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| WebView2 endpoint discovery is fragile (Excel must be launched with `--remote-debugging-port`, no public API to query) | High | High | v1 requires manual launch + documented Excel command line; Phase 2 only probes :9222. Auto-launch deferred to a later phase that wraps `office-addin-debugging` like the TS repo does. |
| Wrong target selected when multiple WebView2 instances are alive | Medium | High | Default heuristic + explicit `cdp.selectTarget`; `status` tool exposes current selection; tests cover the multi-target case. |
| Office.js promise never resolves (e.g. `context.sync()` deadlock) | Medium | Medium | Per-call timeout (default 30s); `Runtime.evaluate` uses `awaitPromise:true`; on timeout, return `category=timeout, retryable=true` and leave session intact. |
| Execution context destroyed by navigation/reload mid-call | Medium | Medium | Subscribe to `executionContextDestroyed`, invalidate cache, retry once with fresh context before surfacing error. |
| Daemon session goes stale silently (Excel closed without disconnect event) | Medium | Medium | Cheap liveness ping (`Runtime.evaluate "1"`) on first call after configurable idle window; reconnect on failure. |
| Windows-only behavior leaks into shared packages | Low | Medium | `_windows.go` / `_other.go` build tags strictly inside `internal/webview2`; nothing else may import OS-specific APIs. |
| Tool-call output stability — agents code against the envelope | High (over time) | High | Versioned envelope (`diagnostics.envelopeVersion`); golden-JSON tests gate any change; documented in `docs/tool-contracts.md` with a "stable since v0.1" mark per field. |
| Hand-rolled CDP client misses an edge case puppeteer handles (e.g. binary frames, large messages) | Medium | Medium | Borrow conformance ideas from chromedp's transport tests; fuzz the message pump; cap message size and surface clear errors. |

---

## 11. Open questions (still to confirm before/during build)

These do not block writing the plan but should be resolved in Phase 0/1:

1. **Final binary name** — `office-addin-mcp` (matches repo) vs shorter `oamcp` vs Windows-friendly `OfficeAddinMcp.exe`?
2. **Package import path / module owner** — github org name to use in `go mod init`.
3. **Daemon socket file location on Windows** — `%LOCALAPPDATA%\office-addin-mcp\daemon.json` (proposed) vs `%APPDATA%`.
4. **Default daemon port** — fixed `45931` (per prompt example) vs ephemeral with port written to socket file. Ephemeral is safer; fixed is friendlier for ad-hoc curl.
5. **`excel.runScript` semantics** — accept arbitrary JS body the agent supplies and run it inside `Excel.run`, or limit to a curated set of named scripts? Affects security posture.
6. **Telemetry** — TS repo has Clearcut + `tiktoken` token counting. v1 plan: none. Confirm.
7. **Auto-launch parity** — the TS repo's `excel_launch_addin` shells out to `office-addin-debugging`. Do we want this in v1, or defer to a Phase 7?
8. **Logging format** — text (default) vs JSON-lines for daemon mode. JSON-lines is friendlier for agents reading logs.

---

## Critical files to create (Phase 0–1 surface)

- `cmd/office-addin-mcp/main.go`
- `internal/cli/{call,serve,daemon,list_tools,status,root}.go`
- `internal/config/config.go`
- `internal/logging/logging.go`
- `internal/cdp/{connection,client,target,runtime,page,dom}.go`
- `internal/webview2/{discovery_windows,discovery_other,attach}.go`
- `internal/session/{manager,session,cache}.go`
- `internal/tools/{registry,schema,dispatcher,result}.go`
- `internal/tools/cdp/{evaluate,targets}.go`
- `internal/officejs/{executor,payloads,serialization}.go`
- `internal/js/_preamble.js` and one payload per `excel.*` tool
- `docs/{architecture,tool-contracts,migration-notes}.md`
- `testdata/golden/<tool>.json`

## Verification (end-to-end)

After Phase 4:
1. Launch Excel with `--remote-debugging-port=9222` and load a sample add-in.
2. `office-addin-mcp call --tool cdp.getTargets` returns the WebView2 target.
3. `office-addin-mcp call --tool excel.listWorksheets` returns the workbook's sheets in the envelope.
4. `office-addin-mcp call --tool excel.readRange --param '{"sheet":"Sheet1","address":"A1:D10"}'` returns values matching the workbook.
5. Re-run #4 ten times against `office-addin-mcp daemon` and confirm `diagnostics.cdpRoundTrips` drops after the first call (session reuse working).
6. Kill Excel, relaunch, re-run #4 — daemon reconnects transparently within reconnect budget; envelope is `ok:true` with no client-visible error.
