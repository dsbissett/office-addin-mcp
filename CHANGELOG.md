# Changelog

## Unreleased

### Added

- **Windows WebView2 endpoint scan + `addin.status` aggregator.** The
  Windows discovery scan was a stub (`scan_windows.go` returning
  `ErrNotFound`), so a user who launched Excel without passing
  `--browser-url` would always fall through to the default :9222 probe and
  silently fail when Excel was on a non-default port. There was also no
  one-shot way to ask the bridge "is everything ready?" — agents had to
  chain `addin.detect` + `addin.listTargets` + `addin.contextInfo` and
  diff the results. Reasoning: a single status call with structured
  recoveryHints[] eliminates a multi-tool dance, and the scan turns a
  "manual port flag" gotcha into an automatic discovery on Windows.
  Concretely:
  - `internal/webview2/scan_windows.go` — replaced the stub with a real
    WebView2 scan. Excel inherits `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`
    as an env var, but the child `msedgewebview2.exe` processes parse it
    onto their command lines, so the scan shells out to
    `wmic process where "name='msedgewebview2.exe'" get CommandLine
    /format:list`, regex-extracts every `--remote-debugging-port=N`,
    deduplicates, and probes `http://127.0.0.1:N/json/version` via the
    existing `cdp.ResolveBrowserWSURL`. Returns the first responding
    endpoint or `ErrNotFound` if `wmic` is missing / nothing's running.
    The 5-second timeout on the wmic invocation prevents a slow box from
    blocking the discovery ladder for long.
  - `internal/webview2/scan_windows_test.go` *(new, build-tagged)* —
    tests the parser against representative `wmic /format:list` output:
    one process with two distinct ports, dedup of repeats, and an
    out-of-range / missing-flag scrub.
  - `internal/tools/addintool/status.go` *(new)* — `addin.status` tool
    (NoSession). Probes the configured endpoint via `webview2.Discover`,
    dials and calls `Target.getTargets`, classifies via
    `addin.ClassifyTargets`, and returns
    `{endpoint{source,browserUrl,wsUrl,reachable,error}, manifest{loaded,
    id,displayName,path,hosts}, targets[], recoveryHints[]}`. Always
    returns `tools.OK` — failures are encoded inside the payload so the
    agent can read both the reachability state and the recovery hint in
    one call instead of branching on `envelope.error`. RecoveryHints
    cover: unreachable endpoint → call `addin.ensureRunning`; missing
    manifest → call `addin.detect`; empty target list → taskpane may not
    be open yet.
  - `internal/tools/runtime.go` — added `RunEnv.Endpoint webview2.Config`.
    NoSession tools previously had no way to read the configured
    endpoint, which mattered because `addin.status` is the
    "is the endpoint reachable?" tool and needs to probe whatever the
    user/server resolved.
  - `internal/tools/dispatcher.go` — populates `env.Endpoint = req.Endpoint`
    on both the NoSession and pooled-session paths.
  - `internal/tools/addintool/register.go` — registers `Status()`.
  - `internal/tools/addintool/addintool_test.go` — extended the registry
    smoke test to assert `addin.ensureRunning` and `addin.status` show up;
    new `TestStatus_UnreachableEndpoint` runs the tool against
    `http://127.0.0.1:1` and asserts `reachable=false`,
    a non-empty `Endpoint.Error`, and a recoveryHint that mentions
    `addin.ensureRunning`.

- **`addin.ensureRunning` tool + `--launch-excel` startup flag.** Bringing
  Excel up with `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=9222"`
  and then sideloading the manifest by hand was the single biggest first-run
  friction point — agents that hit a closed Excel had to chain `addin.detect` +
  `addin.launch` and figure out from text which step was needed. Auto-launch
  is now opt-in via either entry point. Reasoning: probe-first means we never
  spawn Excel when one is already reachable, so the new behavior is safe to
  enable by default in a script while still respecting an explicit
  `--browser-url` / `--ws-endpoint`. Concretely:
  - `internal/launch/launcher.go` — new `LaunchIfNeeded(ctx, project, opts)`
    helper that probes `http://localhost:<port>/json/version` first (using
    the existing `ProbeCDPEndpoint`) and only delegates to `LaunchExcel`
    when the probe fails. Returns a `(result, source, err)` triple where
    `source` is `"preexisting"` or `"launched"` so callers can surface the
    distinction without needing to know which path ran.
  - `internal/tools/addintool/ensurerunning.go` *(new)* — `addin.ensureRunning`
    tool. Probes the configured port; if reachable, returns
    `{source: "preexisting", cdpUrl, manifestPath}` without spawning. If not
    reachable and the project was detected from `cwd`, runs the same
    `office-addin-debugging` path as `addin.launch`, then calls
    `RunEnv.SetEndpoint`/`SetManifest` so subsequent tool calls route to the
    new Excel. When detection fails, returns `addin_not_found` with a
    `RecoveryHint` pointing at `addin.detect` and
    `Details.recoverableViaTool: "addin.detect"`. `LaunchError`-wrapped
    failures get per-reason `RecoveryHint`s
    (`unsupported-platform` → "WebView2 sideloading is Windows-only…",
    `launcher-missing` → "install office-addin-debugging…",
    `port-already-configured` → "unset WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS…",
    `cdp-not-ready` / `dev-server-not-ready` → "retry with longer timeout").
  - `internal/tools/addintool/register.go` — registers `EnsureRunning()`.
  - `cmd/office-addin-mcp/main.go` — new `--launch-excel` flag plus an
    `autoLaunchExcel` helper. When set AND neither `--browser-url` nor
    `--ws-endpoint` was supplied, the binary detects the project from
    process cwd and calls `LaunchIfNeeded` before starting the MCP server,
    threading the resulting `cdpUrl` into `webview2.Config.BrowserURL`.
    Failures are logged via `slog.Warn` rather than fatal — the server
    still starts so the agent can call `addin.ensureRunning` interactively
    or fall back to manual launch.
  - `internal/launch/launcher_test.go` *(new)* — covers the
    "preexisting endpoint" path (httptest stub on a real local port) and
    the "no probe + nil project" guard rail.
  - `CLAUDE.md` — replaced the "Auto-launch is not implemented; do not add
    it without the user asking" note with an entry describing the opt-in
    `--launch-excel` flag and `addin.ensureRunning` tool, with the
    instruction not to auto-launch unconditionally and to honor explicit
    endpoint flags.

### Changed

- **Actionable error envelopes (recoveryHint + standard Details keys).**
  `EnvelopeError` previously surfaced a one-line `message` plus a free-form
  `details` map, which forced agents to parse English to figure out how to
  recover. Reasoning: an AI client that hits
  `code: "session_acquire_failed"` with no further context cannot tell
  whether to retry, call `addin.launch`, or back off — the dispatcher knew
  the answer locally but threw it away. Concretely:
  - `internal/tools/result.go` — added
    `EnvelopeError.RecoveryHint` (one-sentence English suggestion, omitted
    when empty) and documented six standard `Details` keys (`probedEndpoint`,
    `recoverableViaTool`, `cdpError`, `lastKnownTargets`, `manifestUrl`,
    `expectedUrlPattern`) that tools should populate whenever the data is
    locally available. Bumped `EnvelopeVersion` from `v0.3` to `v0.4`.
  - `internal/session/session.go` — exported sentinel errors
    `ErrReconnectBudgetExhausted` and `ErrDialFailed` so callers can branch
    with `errors.Is` instead of substring-matching the wrapped message. The
    dial-failure wrap uses `fmt.Errorf("%w: %w", ErrDialFailed, err)` so
    `errors.Is(ctx.DeadlineExceeded)` still fires when the dial timed out.
  - `internal/tools/dispatcher.go` — the single
    `session_acquire_failed` branch is replaced by a `classifyAcquireErr`
    helper that returns one of four codes
    (`session_reconnect_budget_exhausted`, `session_acquire_timeout`,
    `session_dial_failed`, `session_acquire_failed`), each with a
    code-specific `RecoveryHint` plus `Details["probedEndpoint"]` and (for
    the actionable cases) `Details["recoverableViaTool"] = "addin.launch"`.
  - `internal/tools/runtime.go` — `ClassifyCDPErr` now uses
    `errors.As` to pull the structured `*cdp.RemoteError` out of the chain,
    surfacing `{code, message, data}` as `Details["cdpError"]` so an agent
    can branch on the CDP-level code instead of regexing the message.
  - `internal/tools/addintool/errors.go` — `mapPayloadError` populates
    `RecoveryHint` for the well-known Office.js codes thrown by
    `internal/js/_preamble.js`: `office_unavailable` / `excel_unavailable`,
    `office_ready_failed` / `office_ready_timeout`, and
    `requirement_unmet` / `requirement_check_failed`.
  - `internal/session/session_test.go` — the existing reconnect-budget
    test now asserts via `errors.Is(err, ErrDialFailed)` and
    `errors.Is(err, ErrReconnectBudgetExhausted)` rather than substring
    matching, validating the sentinel-error contract.
  - `internal/tools/dispatcher_test.go` — new
    `TestEnvelopeErrorRecoveryHints` table-driven test covers the four
    acquire-failure modes and asserts code, category, `probedEndpoint`,
    `recoverableViaTool`, and recoveryHint substring.
  - `internal/tools/testdata/golden/{success,validation_error,cdp_error,timeout,unknown_tool}.json`
    bumped to `v0.4`.

- **Structured logging + per-call request correlation.** The binary
  now configures `log/slog` with a JSON handler at startup. The
  dispatcher (`internal/tools/dispatcher.go`) generates a
  cryptographically-random 16-hex-char request id at the top of every
  `Dispatch`, threads it through the call's `context.Context` via the
  new `internal/log` helper, copies it into the envelope's
  `diagnostics.requestId`, and emits `dispatch.start`/`dispatch.end`
  debug log lines tagged with it. Downstream layers can pick the id
  off ctx — `internal/cdp/connection.go` `Send` already does, so each
  CDP round-trip can be tied back to one tool call without having to
  reverse-engineer the call from a wall-clock window. Reasoning: the
  server was previously silent unless `--log-file` was set, and even
  then it wrote unstructured `fmt.Fprintf` lines with no correlation
  id, which made it impossible to tell which CDP send belonged to
  which tool call when an MCP client issued anything in parallel.
  Concretely:
  - `internal/log/log.go` *(new)* — leaf package (stdlib only) with
    `WithRequestID`/`RequestID` for ctx-scoped ids and a
    `RecoverGoroutine(name)` defer-friendly panic catcher that logs at
    `ERROR` level via slog.
  - `internal/tools/result.go` — added `Diagnostics.RequestID`. Bumped
    `EnvelopeVersion` from `v0.2` to `v0.3` (per-call hex correlation
    id stamped by the dispatcher and threaded through ctx).
  - `internal/tools/testdata/golden/{success,validation_error,cdp_error,timeout,unknown_tool}.json`
    bumped to `v0.3`. `canonicalize` in
    `internal/tools/dispatcher_test.go` now zeroes `RequestID` so the
    randomized id does not break golden diffs.
  - `cmd/office-addin-mcp/main.go` — initializes `slog.SetDefault`
    with `slog.NewJSONHandler` writing to `--log-file` (or stderr) at
    the level chosen by the new `--log-level` flag (`debug|info|warn|
    error`, default `info`). The mcp-server-exit error message now
    goes through `slog.Error` rather than a raw `fmt.Fprintf`.
  - `internal/tools/dispatcher_test.go` — new `TestDispatchStampsRequestID`
    asserts `Diagnostics.RequestID` is 16 hex chars and unique across
    five back-to-back calls.

- **Panic recovery on every long-lived goroutine.** The CDP
  `readLoop` and the launch package's `drainPipe`/`waitChild`
  goroutines plus the session `Manager.gcLoop` previously had no
  `defer recover()`, so a malformed frame, unexpected EOF in a child
  pipe, or a stray nil deref would silently kill the goroutine and
  leave the surrounding subsystem in an undefined state. Reasoning:
  for a production AI bridge, "the server stopped responding" with no
  log line is the worst possible failure mode — recover() costs
  essentially nothing and turns the panic into a single structured
  log entry plus a clean shutdown of the affected resource.
  Concretely:
  - `internal/cdp/connection.go` `readLoop` — `defer` block now
    catches any panic, logs it via `slog.Error` with the goroutine
    name and panic value, and calls `closeWithErr` so pending
    requesters get `ErrClosed` instead of hanging forever. Also added
    a `slog.Debug("cdp.send", ...)` tagged with the request id (when
    one is on ctx) so per-CDP-call logs correlate with the dispatcher's
    `dispatch.start`/`dispatch.end` lines.
  - `internal/session/manager.go` `gcLoop` — wraps the loop body in
    `defer log.RecoverGoroutine("session.gcLoop")`.
  - `internal/launch/devserver.go` `drainPipe` and the anonymous
    `cmd.Wait()` goroutine inside `waitChild` — same wrap.

- **Documented stdio-only contract.** `cmd/office-addin-mcp/main.go` already
  rejects positional arguments and the historic `call` / `daemon` /
  `serve --stdio` subcommands have been gone since the MCP-over-stdio
  rewrite, but the README continued to advertise them and the version
  metadata across `mcp.json`, the npm package files, and the README
  banner had drifted. Reasoning: misleading docs cause new users to
  type a command and immediately hit `unexpected argument`, eroding
  trust on first contact, and the version disagreement makes it
  impossible to tell at a glance which tag a checkout corresponds to.
  Concretely:
  - `README.md` now describes the binary as MCP-over-stdio only;
    removed the "Daemon mode" and "Stdio mode (MCP protocol)" sections,
    the broken `office-addin-mcp call …` Quick-start examples, the
    `--no-daemon` / `--idle-timeout` flag rows, and the Subcommands
    table. Added the missing `--ws-endpoint`, `--log-file`, and
    `--version` flags. Replaced the hard-coded `v0.1.0` banner with a
    pointer to the GitHub Releases page so the doc no longer goes
    stale on every tag.
  - `npm/main/package.json`, `npm/win32-x64/package.json`,
    `npm/darwin-x64/package.json`, `npm/darwin-arm64/package.json`,
    `npm/linux-x64/package.json`, `npm/linux-arm64/package.json` —
    versions (and `optionalDependencies` pins in `npm/main`) bumped
    from `0.1.0`/`0.1.1` to `0.2.0` to match `mcp.json`. The release
    workflow re-stamps these from the tag at publish time, so the
    checked-in values are informational, but consistent baseline state
    keeps `git diff` against a release tag readable.

### Added

- **Release workflow drift check.**
  `.github/workflows/release.yml` now runs a "Contract drift check"
  step before the existing version comparison. It (a) greps `README.md`
  for the four removed subcommand invocations
  (`office-addin-mcp call`, `office-addin-mcp daemon`,
  `office-addin-mcp serve --stdio`, `office-addin-mcp list-tools`)
  and (b) asserts every checked-in
  `npm/<platform>/package.json` and `npm/main/package.json` version
  agrees with `mcp.json`. Reasoning: the prior behavior allowed a tag
  to ship even when the README still advertised dead subcommands or
  one of the npm packages had been hand-edited but its peers missed
  — this check fails fast at the start of the release pipeline so the
  drift is caught before publishing to npm or the MCP registry.
- **`page.consoleLog`, `page.networkLog`, `page.networkBody`.** Three
  event-buffer-backed inspection tools for the high-level surface. The
  first call against a target subscribes to the relevant CDP events
  (`Runtime.consoleAPICalled` / `Runtime.exceptionThrown` /
  `Log.entryAdded` for console; `Network.*` for network) and starts a
  pump goroutine that drains them into a per-target ring buffer kept on
  the session. Subsequent calls drain accumulated entries via a
  monotonic `seq` cursor (`sinceSeq` → `lastSeq`). Bounded at 1000
  entries by default, overridable via `maxBuffer`. Buffers are
  per-`cdpSessionId`, so flipping pages with `pages.select` preserves
  prior-target output; everything is cleared on CDP reconnect.
  `page.networkLog` correlates `requestWillBeSent` /
  `responseReceived` / `loadingFinished` / `loadingFailed` into one
  record per completed request, with optional `includeHeaders` and
  status / URL / failed-only filters. `page.networkBody` fetches the
  response body for a logged `requestId`, capped at 5 MiB.
- **`cdp.Connection.SubscribeMethods`.** Multi-method subscribe form
  used by the network pump to preserve cross-method ordering
  (`requestWillBeSent` before `responseReceived` for the same
  `requestId`) — single-method subscribe channels couldn't guarantee
  this under a multi-channel `select`.
- **Phase 6: raw CDP gated behind `--expose-raw-cdp`.** The default
  `tools/list` advertises only the high-level Office add-in surface
  (`addin.*`, `pages.*`, `page.*`, `excel.*`, plus the interaction tools
  registered as `page.click` / `page.fill` / `page.hover` /
  `page.typeText` / `page.pressKey`). Pass `--expose-raw-cdp` (or set
  `OFFICE_ADDIN_MCP_EXPOSE_RAW_CDP=1`) to also register the ~411
  code-generated `cdp.*` tools and the `cdp.selectTarget` cache primer.
- **Phase 5: Excel tool surface expansion (11 → 37).** New `excel.*` tools
  cover workbook (`workbookInfo`, `calculationState`, `listNamedItems`,
  `customXmlParts`, `settingsGet`), worksheet (`worksheetInfo`,
  `listComments`, `listShapes`), range (`activeRange`, `usedRange`,
  `rangeProperties`, `rangeFormulas`, `rangeSpecialCells`, `findInRange`,
  `listConditionalFormats`, `listDataValidations`), tables (`listTables`,
  `tableInfo`, `tableRows`, `tableFilters`), charts (`listCharts`,
  `chartInfo`, `chartImage`), and PivotTables (`listPivotTables`,
  `pivotTableInfo`, `pivotTableValues`). Read tools that materialize 2D
  grids cap output at 1000 cells and flag truncation. `excel.chartImage`
  payload returns `{mimeType, data}`, surfaced as an MCP `ImageContent`
  block by the adapter.
- **Code-generated CDP tool surface (~411 tools across 18 domains).**
  Every method named in `CdpProtocols.md` and present in the vendored
  `browser_protocol.json` / `js_protocol.json` is now exposed as
  `cdp.<lowerDomain>.<lowerMethod>`. See
  [docs/cdp-tools.md](docs/cdp-tools.md) for the index.
- `cmd/gen-cdp-tools` — code generator driven by `cdp/manifest.yaml`
  (policy overlay) plus the vendored protocol JSON. Deterministic
  output (sorted iteration + in-process `go/format`); drift test
  enforces no-diff against checked-in output.
- `scripts/build_manifest.py` — regenerates the manifest from
  `CdpProtocols.md` with hand-curated DANGEROUS / BINARY_FIELD /
  BROWSER_SCOPED / AUTO_ENABLE tables.
- Vendored Chrome devtools-protocol JSON at SHA
  `470fb6a42cbcaf446b516d8fc7738f9723cba5fc` (r1621552, 2026-04-28),
  pinned in `cdp/protocol/VERSION`.
- Lazy domain enabling — `Session.EnsureEnabled` issues
  `<Domain>.enable` exactly once per `(cdpSessionID, domain)` pair;
  cleared on reconnect (`dropConnLocked`). `RunEnv.EnsureEnabled` is
  the dispatcher-side hook generated tools call before their first
  command.
- `--allow-dangerous-cdp` flag (and `OAMCP_ALLOW_DANGEROUS_CDP=1`)
  on `call` / `serve` / `daemon`. Generated dangerous tools
  (`cdp.browser.crash`, `cdp.runtime.terminateExecution`, etc.)
  refuse with `category=unsupported, code=dangerous_disabled` unless
  set. Process-wide on the daemon; no per-call override.
- Binary `outputPath` codegen for `cdp.page.captureScreenshot`,
  `cdp.page.printToPDF`, `cdp.page.captureSnapshot`. Setting the
  param decodes the base64 result to disk and returns
  `{path, sizeBytes, mimeType}` instead of raw bytes.
  `tools.WriteBinaryFieldOutput` is the shared helper.

### Changed

- `cdp.evaluate`, `cdp.getTargets`, and `browser.navigate` are
  **removed**. Use `page.evaluate`, `pages.list`, and `page.navigate`
  instead. Power users who still want raw CDP access can run with
  `--expose-raw-cdp` and call `cdp.runtime.evaluate`,
  `cdp.target.getTargets`, and `cdp.page.navigate` directly.
  `cdp.selectTarget` remains hand-written (primes the per-session
  selector cache; no direct CDP equivalent) and is now also gated
  behind `--expose-raw-cdp`.
- `.golangci.yml` migrated from v1 to v2 syntax with `errcheck`
  exclusions for `fmt.Fprint*` and `(io.Closer).Close` (idiomatic
  no-recovery patterns). One pre-existing gofumpt fix in
  `internal/cdp/runtime_test.go` (collapsed two `var _ = …` into a
  `var (…)` block).

### Tests

- Generator: `TestGolden` + `TestDeterministic` (fixture-based);
  `TestLiveManifestDrift` (replaces `go generate && git diff`).
- Naming: `TestGeneratedToolNamesMatchPattern` enforces
  `^cdp\.[a-z][a-zA-Z]*\.[a-z][a-zA-Z]*$` on every generated tool.
- Lazy enable: `TestEnsureEnabledOnceAcrossCalls`,
  `TestEnsureEnabledPerCDPSession`,
  `TestEnsureEnabledClearedOnReconnect`.
- P6 codegen paths: `TestDangerousRefusedWithoutFlag`,
  `TestBinaryOutputPathWritesFile`,
  `TestBinaryOutputPathOmittedReturnsRaw`.

### Notes

- Refreshing the vendored protocol JSON is a breaking change at the
  result-shape boundary — generated tools pass Chrome's response
  through verbatim. See
  [docs/migration-notes.md](docs/migration-notes.md#refreshing-the-protocol)
  for the procedure.

## v0.1.0 — 2026-04-29

First tagged release. Implements PLAN.md Phases 0 through 6.

### Added

- Hand-rolled CDP client over `gorilla/websocket` with id-correlated
  request/response and method-keyed event subscribe
  (`internal/cdp/connection.go`).
- WebView2 endpoint discovery with priority ladder: explicit
  ws-endpoint > explicit browser-url > default `http://127.0.0.1:9222`
  > Windows scan stub (`internal/webview2/`).
- Tool registry with JSON-Schema-validated boundary
  (`santhosh-tekuri/jsonschema/v5`) and uniform `{ok, data, error,
  diagnostics}` envelope versioned via `EnvelopeVersion`
  (`internal/tools/`).
- 15 tools across three domains:
  - `cdp.evaluate`, `cdp.getTargets`, `cdp.selectTarget`
  - `browser.navigate`
  - `excel.readRange`, `excel.writeRange`, `excel.listWorksheets`,
    `excel.getActiveWorksheet`, `excel.activateWorksheet`,
    `excel.createWorksheet`, `excel.deleteWorksheet`,
    `excel.getSelectedRange`, `excel.setSelectedRange`,
    `excel.createTable`, `excel.runScript`
- Office.js execution stack: 11 embedded payloads + a preamble
  (`__officeError`, `__ensureOffice`, `__requireSet`, `__runExcel`)
  with structured error reporting (`internal/officejs/`,
  `internal/js/`).
- Persistent session pool with sticky selector cache, sliding-window
  reconnect budget (3 in 60s), and idle GC (`internal/session/`).
  Session reuse drops `cdpRoundTrips` from ~3 to 1 after the first
  call.
- Daemon mode: HTTP/1.1 server on `127.0.0.1` with bearer-token auth,
  endpoints `/v1/{health,call,list-tools,status}`, socket file at
  `os.UserCacheDir()/office-addin-mcp/daemon.json` mode 0600
  (`internal/daemon/`).
- `serve --stdio` subcommand for stdio-mode hosts.
- `call` subcommand auto-routes to a healthy daemon when the socket
  file is present; `--no-daemon` forces in-process dispatch.
- `list-tools` subcommand emits the registered catalog with JSON
  Schemas.
- Documentation: README quick-start, `docs/architecture.md`,
  `docs/tool-contracts.md`, `docs/migration-notes.md`.

### Envelope

`v0.2` — `sessionId` is the user/Phase-5 session id; `cdpSessionId`
carries the CDP flatten session id; `cdpRoundTrips` diagnostic added.

### Tests

50+ unit and integration tests including:

- CDP message correlation, exception unwrap, ctx timeout, post-close
  failure, event dispatch.
- Endpoint discovery priority ladder.
- Tool registry duplicate / empty-name / nil-Run / bad-schema; JSON
  Schema validation (required, additionalProperties, null-as-empty,
  decode errors).
- Golden-JSON envelope shapes for success / validation_error /
  cdp_error / timeout / unknown_tool.
- Office.js executor: success unwrap, OfficeError unwrap with
  debugInfo, protocol exception, transport error pass-through, U+2028
  escape via `json.Marshal` default.
- Per-tool integration tests against a fakeBrowser (cdp.evaluate,
  cdp.getTargets filter, cdp.selectTarget, browser.navigate,
  excel.readRange success + Office.js error, excel.runScript args).
- Session: sticky cache hit/miss, reconnect-budget exhaustion,
  selection cache cleared on reconnect, idle GC, Drop closes
  connection.
- Daemon: health + auth (401 without bearer), Stop removes socket
  file, **10 sequential `call` invocations against the daemon use
  exactly 1 WS dial** with steady-state cdpRoundTrips=1.

### Known limitations

- No auto-launch of Excel via `office-addin-debugging` (PLAN.md §11
  Q7); launch Excel manually with `--remote-debugging-port=9222`.
- WebView2 user-data-dir scanning is stubbed (PLAN.md §10).
- `excel.runScript` ships the permissive variant accepting arbitrary
  async JS bodies (PLAN.md §11 Q5).
- No telemetry / token-counting (PLAN.md §11 Q6).
- Per-tool golden response JSON against a real fixture workbook is
  populated manually per the checklist in `docs/migration-notes.md`.
