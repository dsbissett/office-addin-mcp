# Migration notes — TypeScript `excel-webview2-mcp` → Go `office-addin-mcp`

This is the parity contract for v0.1.0. For the on-the-wire envelope
spec see [tool-contracts.md](tool-contracts.md); for the package map
see [architecture.md](architecture.md).

## Why a rewrite

The TypeScript repo ships a working stdio MCP server forked from
`chrome-devtools-mcp`, but it carries fork weight: a ~200 MB
`chrome-devtools-frontend` dependency, puppeteer's full browser
abstraction, 70+ tools (most inherited from chrome-devtools rather than
designed for Office), and a process model that re-pays the connection
cost on every invocation.

The Go binary is a redesign informed by that repo, not a port:

- Hand-rolled CDP client over `gorilla/websocket` — no chromedp / puppeteer.
- Office.js as **payloads injected through CDP**, not a runtime to
  integrate with: `internal/js/*.js` files embedded via `//go:embed`.
- Stable envelope (`{ok, data, error, diagnostics}`) versioned via
  `EnvelopeVersion`.
- Daemon mode amortizes attach cost; one-shot still works for ad-hoc CLI use.

## Tool parity

| TS tool                       | Go tool                       | Status   | Notes                                                          |
| ----------------------------- | ----------------------------- | -------- | -------------------------------------------------------------- |
| `cdp_evaluate`                | `page.evaluate`               | ported, **renamed** | Replaces the removed `cdp.evaluate` deprecation alias. For raw access use `cdp.runtime.evaluate` under `--expose-raw-cdp`. |
| `cdp_get_targets`             | `pages.list`                  | ported, **renamed** | Replaces the removed `cdp.getTargets` deprecation alias. For raw access use `cdp.target.getTargets` under `--expose-raw-cdp`. |
| `cdp_select_target`           | `cdp.selectTarget`            | ported, **gated** | Primes the per-session selector cache. Now hidden behind `--expose-raw-cdp`; high-level callers should use `pages.select`. |
| `browser_navigate`            | `page.navigate`               | ported, **renamed** | Replaces the removed `browser.navigate` deprecation alias. For raw access use `cdp.page.navigate` under `--expose-raw-cdp`. |
| `excel_read_range`            | `excel.readRange`             | ported   |                                                                |
| `excel_write_range`           | `excel.writeRange`            | ported   | `anyOf` requires `values` / `formulas` / `numberFormat`.       |
| `excel_list_worksheets`       | `excel.listWorksheets`        | ported   |                                                                |
| `excel_get_active_worksheet`  | `excel.getActiveWorksheet`    | ported   |                                                                |
| `excel_activate_worksheet`    | `excel.activateWorksheet`     | ported   | Requires ExcelApi 1.7.                                         |
| `excel_create_worksheet`      | `excel.createWorksheet`       | ported   |                                                                |
| `excel_delete_worksheet`      | `excel.deleteWorksheet`       | ported   |                                                                |
| `excel_get_selected_range`    | `excel.getSelectedRange`      | ported   |                                                                |
| `excel_set_selected_range`    | `excel.setSelectedRange`      | ported   |                                                                |
| `excel_run_script`            | `excel.runScript`             | ported   | Permissive variant — see PLAN.md §11 Q5.                       |
| `excel_create_table`          | `excel.createTable`           | ported   |                                                                |
| `excel_launch_addin`          | —                             | deferred | Auto-launch via `office-addin-debugging` — PLAN.md §11 Q7.     |
| `lighthouse_*`                | —                             | dropped  | Out of scope for v1.                                           |
| `performance_*`               | —                             | dropped  | Out of scope.                                                  |
| `screencast_*`                | —                             | dropped  | Out of scope.                                                  |
| `take_memory_snapshot`        | —                             | dropped  | Out of scope.                                                  |
| in-page DevTools interop      | —                             | dropped  | Out of scope.                                                  |
| `emulate_*`                   | —                             | dropped  | Not relevant to Office add-in automation.                      |

## Renames

- `snake_case` → `<domain>.<verbNoun>`. Example: `excel_read_range` →
  `excel.readRange`.
- `McpResponse` markdown → `{ok, data, error, diagnostics}` JSON
  envelope (versioned via `diagnostics.envelopeVersion`).
- TS `requestId` → Go envelope has no request id (the dispatcher's
  callers correlate by their own ordering).

## TS-isms retired

- `zod` schemas → JSON Schema files (one per tool) compiled by
  `santhosh-tekuri/jsonschema/v5`.
- Per-tool ad-hoc result shaping → uniform envelope.
- Markdown-formatted `text/plain` returns → structured `data` field.
- `tiktoken` token counting → not present.
- Clearcut telemetry → not present (PLAN.md §11 Q6).

## Daemon and stdio (new in Go)

The TS server is stdio-only. The Go binary adds:

- A long-lived **daemon** on `127.0.0.1` with bearer-token auth and a
  well-known socket file. `call` autoroutes to it.
- **`serve --stdio`** for stdio-mode hosts. Same dispatcher path as the
  daemon — sessions persist for the stream.

This is what makes `cdpRoundTrips` drop after the first call: the
session.Manager keeps the connection open and a sticky selector cache
hot. The TS server re-paid `Target.getTargets` + `attachToTarget` on
every invocation.

## Manual Excel acceptance checklist

These need a real Excel + a sample add-in. Launch Excel with
`--remote-debugging-port=9222` (set
`WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` before launch), exercise each
tool through `office-addin-mcp call`, and capture the envelope into
`testdata/golden/excel/<tool>.json` as a regression baseline.

Workbook fixture: `Sheet1` populated with values in `A1:D10`, plus a
sheet named `Hidden` set to `visibility: hidden`.

- [ ] `excel.listWorksheets` — returns `Sheet1` and `Hidden`;
  `Hidden.visibility = "Hidden"`.
- [ ] `excel.getActiveWorksheet` — `name = "Sheet1"`.
- [ ] `excel.activateWorksheet` `{name: "Hidden"}` — succeeds; subsequent
  `getActiveWorksheet` returns `"Hidden"`.
- [ ] `excel.createWorksheet` `{name: "Tmp"}` — creates and returns it.
- [ ] `excel.deleteWorksheet` `{name: "Tmp"}` — removes it.
- [ ] `excel.readRange` `{address: "A1:D10"}` — values match the fixture.
- [ ] `excel.writeRange` `{address: "F1:F3", values: [[1],[2],[3]]}` —
  `excel.readRange` round-trips.
- [ ] `excel.getSelectedRange` — returns the currently selected cell.
- [ ] `excel.setSelectedRange` `{address: "B2:C4"}` — `getSelectedRange`
  echoes back `B2:C4`.
- [ ] `excel.createTable` `{address: "A1:D10", name: "ParityTable"}` —
  table appears in workbook.
- [ ] `excel.runScript` `{script: "const w = context.workbook;
  w.load('name'); await context.sync(); return w.name;"}` — returns the
  workbook name.

For the daemon-mode acceptance ("ten calls, one attach"), see
[PLAN.md](../PLAN.md) §7 Phase 5 — automated coverage via
`internal/daemon/server_test.go::TestDaemon_TenCallsReuseOneConnection`.

## CDP tool generation (post-v0.1)

Beyond the TS parity surface above, the Go binary now ships ~411
code-generated CDP tools across 18 domains. These have no TS analogue —
the TS server only re-exposed Chrome DevTools tools through chromedp,
not the raw protocol. See [tool-contracts.md](tool-contracts.md) and
[cdp-tools.md](cdp-tools.md) for the full surface.

The generator is driven by:

- `cdp/protocol/{browser,js}_protocol.json` — Chrome's vendored protocol
  JSON, pinned to the SHA in `cdp/protocol/VERSION`.
- `cdp/manifest.yaml` — policy overlay (allowlist, scope, autoEnable,
  dangerous, binaryField/MimeType).
- `cmd/gen-cdp-tools` — `go generate ./...` entry point.

### Refreshing the protocol

Refreshing the vendored JSON is a **breaking change at the result-shape
boundary**. Each generated tool's `data` is a passthrough of Chrome's
response, so a Chrome-side rename or type change propagates to callers.
When rolling:

1. Update `cdp/protocol/{browser,js}_protocol.json` to the new SHA.
2. Update the SHA / roll number / date in `cdp/protocol/VERSION` and
   the matching constant in `cdp/protocol_test.go::TestVersionFile`.
3. `python scripts/build_manifest.py > cdp/manifest.yaml` to drop
   methods Chrome removed and pick up newly added ones.
4. `go generate ./...` to regenerate the Go files and
   `docs/cdp-tools.md`.
5. `go test ./...` — drift test will fail if step 4 was forgotten;
   schema-compile failures will surface if any generated schema is
   malformed; daemon acceptance test still asserts steady-state
   `cdpRoundTrips=1`.
6. Note the roll in this file's "## CDP tool generation" section as a
   protocol-version entry, and add a CHANGELOG entry under the
   appropriate version.

### Legacy alias removal (Phase 6)

`cdp.evaluate`, `cdp.getTargets`, and `browser.navigate` were removed
in the Phase 6 cutover. The high-level replacements are
`page.evaluate`, `pages.list`, and `page.navigate` respectively. Power
users who want the raw Chrome DevTools surface can run with
`--expose-raw-cdp` (or `OFFICE_ADDIN_MCP_EXPOSE_RAW_CDP=1`) and call
`cdp.runtime.evaluate`, `cdp.target.getTargets`, and `cdp.page.navigate`
directly. The hand-written `cdp.selectTarget` primer is still available
under the same flag for callers that depend on the per-session
selector cache.

## Known divergences from TS

- No auto-launch of Excel via `office-addin-debugging`. Launch Excel
  manually with `--remote-debugging-port` for now (PLAN.md §11 Q7).
- `excel.runScript` accepts an arbitrary async body. Same as the TS
  repo. Tighten via an allowlist later if security posture demands it
  (PLAN.md §11 Q5).
- Telemetry (Clearcut, `tiktoken`) is not present (PLAN.md §11 Q6).
- WebView2 user-data-dir scanning is stubbed; explicit
  `--browser-url`/`--ws-endpoint` is required outside the default :9222
  fallback (PLAN.md §10).
