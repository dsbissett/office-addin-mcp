# Changelog

## Unreleased

### Added

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
