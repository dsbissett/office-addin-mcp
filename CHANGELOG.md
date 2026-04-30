# Changelog

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
